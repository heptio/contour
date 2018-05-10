// Copyright © 2017 Heptio
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package grpc provides a gRPC implementation of the Envoy v2 xDS API.
package grpc

import (
	"context"
	"fmt"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_service_v2 "github.com/envoyproxy/go-control-plane/envoy/service/load_stats/v2"
	"github.com/sirupsen/logrus"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/contour"
)

const (
	// somewhat arbitrary limit to handle many, many, EDS streams
	grpcMaxConcurrentStreams = 1 << 20
)

// NewAPI returns a *grpc.Server which responds to the Envoy v2 xDS gRPC API.
func NewAPI(log logrus.FieldLogger, t *contour.Translator) *grpc.Server {
	opts := []grpc.ServerOption{
		// By default the Go grpc library defaults to a value of ~100 streams per
		// connection. This number is likely derived from the HTTP/2 spec:
		// https://http2.github.io/http2-spec/#SettingValues
		// We need to raise this value because Envoy will open one EDS stream per
		// CDS entry. There doesn't seem to be a penalty for increasing this value,
		// so set it the limit similar to envoyproxy/go-control-plane#70.
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
	}
	g := grpc.NewServer(opts...)
	s := newgrpcServer(log, t)
	v2.RegisterClusterDiscoveryServiceServer(g, s)
	v2.RegisterEndpointDiscoveryServiceServer(g, s)
	v2.RegisterListenerDiscoveryServiceServer(g, s)
	v2.RegisterRouteDiscoveryServiceServer(g, s)
	return g
}

type grpcServer struct {
	logrus.FieldLogger
	count     uint64              // connection count, incremented atomically
	resources map[string]resource // registered resource types
}

func newgrpcServer(log logrus.FieldLogger, t *contour.Translator) *grpcServer {
	return &grpcServer{
		FieldLogger: log,
		resources: map[string]resource{
			clusterType: &CDS{
				cache: &t.ClusterCache,
			},
			endpointType: &EDS{
				cache: &t.ClusterLoadAssignmentCache,
			},
			listenerType: &LDS{
				cache: &t.ListenerCache,
			},
			routeType: &RDS{
				HTTP:  &t.VirtualHostCache.HTTP,
				HTTPS: &t.VirtualHostCache.HTTPS,
				Cond:  &t.VirtualHostCache.Cond,
			},
		},
	}
}

// A resource provides resources formatted as []types.Any.
type resource interface {
	cache

	// TypeURL returns the typeURL of messages returned from Values.
	TypeURL() string
}

func (s *grpcServer) FetchClusters(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return s.fetch(req)
}

func (s *grpcServer) FetchEndpoints(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return s.fetch(req)
}

func (s *grpcServer) FetchListeners(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return s.fetch(req)
}

func (s *grpcServer) FetchRoutes(_ context.Context, req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return s.fetch(req)
}

// fetch handles a single DiscoveryRequest.
func (s *grpcServer) fetch(req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	s.WithField("connection", atomic.AddUint64(&s.count, 1)).WithField("version_info", req.VersionInfo).WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl).WithField("response_nonce", req.ResponseNonce).WithField("error_detail", req.ErrorDetail).Info("fetch")
	r, ok := s.resources[req.TypeUrl]
	if !ok {
		return nil, fmt.Errorf("no resource registered for typeURL %q", req.TypeUrl)
	}
	resources, err := toAny(r, toFilter(req.ResourceNames))
	return &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   resources,
		TypeUrl:     r.TypeURL(),
		Nonce:       "0",
	}, err
}

func (s *grpcServer) StreamClusters(srv v2.ClusterDiscoveryService_StreamClustersServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamEndpoints(srv v2.EndpointDiscoveryService_StreamEndpointsServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamLoadStats(srv envoy_service_v2.LoadReportingService_StreamLoadStatsServer) error {
	return status.Errorf(codes.Unimplemented, "StreamLoadStats Unimplemented")
}

func (s *grpcServer) StreamListeners(srv v2.ListenerDiscoveryService_StreamListenersServer) error {
	return s.stream(srv)
}

func (s *grpcServer) StreamRoutes(srv v2.RouteDiscoveryService_StreamRoutesServer) error {
	return s.stream(srv)
}

type grpcStream interface {
	Context() context.Context
	Send(*v2.DiscoveryResponse) error
	Recv() (*v2.DiscoveryRequest, error)
}

// stream processes a stream of DiscoveryRequests.
func (s *grpcServer) stream(st grpcStream) (err error) {

	// bump connection counter and set it as a field on the logger
	log := s.WithField("connection", atomic.AddUint64(&s.count, 1))

	// set up some nice function exit handling which notifies if the
	// stream terminated on error or not.
	defer func() {
		if err != nil {
			log.WithError(err).Error("stream terminated")
		} else {
			log.Info("stream terminated")
		}
	}()

	ch := make(chan int, 1)
	last := 0
	ctx := st.Context()

	// now stick in this loop until the client disconnects.
	for {

		// first we wait for the request from Envoy, this is part of
		// the xDS protocol.
		req, err := st.Recv()
		if err != nil {
			return err
		}

		// from the request we derive the resource to stream which have
		// been registered according to the typeURL.
		r, ok := s.resources[req.TypeUrl]
		if !ok {
			return fmt.Errorf("no resource registered for typeURL %q", req.TypeUrl)
		}

		// stick some debugging details on the logger, not that we redeclare log in this scope
		// so the next time around the loop all is forgotten.
		log := log.WithField("version_info", req.VersionInfo).WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl).WithField("response_nonce", req.ResponseNonce).WithField("error_detail", req.ErrorDetail)

		// we wait in this loop until we find at least one resource that matches the filter supplied
	streamwait:
		for {
			log.Info("stream_wait")

			// now we wait for a notification, if this is the first time throught the loop
			// then last will be zero and that will trigger a notification immediately.
			r.Register(ch, last)
			select {

			// boom, something in the cache has changed
			case last = <-ch:

				// generate a filter from the request, then call toAny which
				// will get r's (our resource) filter values, then convert them
				// to the types.Any from required by gRPC.
				resources, err := toAny(r, toFilter(req.ResourceNames))
				if err != nil {
					return err
				}

				// if we didn't get any resources because they were filter out
				// or are not present in the cache, then skip the update. This will
				// mean that if Envoy asks EDS for a set of end points that are not
				// present (say during pre-warming) it will stay in pre-warming, rather
				// than receive an result with an empty set of ClusterLoadAssignments.
				if len(resources) == 0 {

					// there were no matching resources, or no resources at all, found
					// so don't send anything back to the caller.
					log.Info("skipping update")
					continue streamwait
				}

				// otherwise, build the response object and stream it back to the client.
				resp := &v2.DiscoveryResponse{
					VersionInfo: "0",
					Resources:   resources,
					TypeUrl:     r.TypeURL(),
					Nonce:       "0",
				}
				if err := st.Send(resp); err != nil {
					return err
				}

				// ok, the client hung up, return any error stored in the context and we're done.
			case <-ctx.Done():
				return ctx.Err()
			}

		}
	}
}

// toAny converts the contens of a resourcer's Values to the
// respective slice of types.Any.
func toAny(res resource, filter func(string) bool) ([]types.Any, error) {
	v := res.Values(filter)
	resources := make([]types.Any, len(v))
	for i := range v {
		value, err := proto.Marshal(v[i])
		if err != nil {
			return nil, err
		}
		resources[i] = types.Any{TypeUrl: res.TypeURL(), Value: value}
	}
	return resources, nil
}

// toFilter converts a slice of strings into a filter function.
// If the slice is empty, then a filter function that matches everything
// is returned.
func toFilter(names []string) func(string) bool {
	if len(names) == 0 {
		return func(string) bool { return true }
	}
	m := make(map[string]bool)
	for _, n := range names {
		m[n] = true
	}
	return func(name string) bool { return m[name] }
}
