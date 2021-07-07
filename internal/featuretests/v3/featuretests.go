// Copyright Project Contour Authors
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

package v3

// provides end to end tests of specific features.

import (
	"context"
	"math/rand"
	"net"
	"sort"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"

	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"github.com/projectcontour/contour/internal/status"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/projectcontour/contour/internal/xds"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	endpointType = resource.EndpointType // nolint:varcheck,deadcode
	clusterType  = resource.ClusterType
	routeType    = resource.RouteType
	listenerType = resource.ListenerType
	secretType   = resource.SecretType
	statsAddress = "0.0.0.0"
	statsPort    = 8002
)

func setup(t *testing.T, opts ...interface{}) (cache.ResourceEventHandler, *Contour, func()) {
	t.Parallel()

	log := fixture.NewTestLogger(t)
	log.SetLevel(logrus.DebugLevel)

	et := xdscache_v3.NewEndpointsTranslator(log)

	conf := xdscache_v3.ListenerConfig{}
	for _, opt := range opts {
		if opt, ok := opt.(func(*xdscache_v3.ListenerConfig)); ok {
			opt(&conf)
		}
	}

	resources := []xdscache.ResourceCache{
		xdscache_v3.NewListenerCache(conf, statsAddress, statsPort),
		&xdscache_v3.SecretCache{},
		&xdscache_v3.RouteCache{},
		&xdscache_v3.ClusterCache{},
		et,
	}

	registry := prometheus.NewRegistry()

	rand.Seed(time.Now().Unix())

	statusUpdateCacher := &k8s.StatusUpdateCacher{}
	eh := &contour.EventHandler{
		IsLeader:      make(chan struct{}),
		StatusUpdater: statusUpdateCacher,
		FieldLogger:   log,
		Sequence:      make(chan int, 1),
		//nolint:gosec
		HoldoffDelay: time.Duration(rand.Intn(100)) * time.Millisecond,
		//nolint:gosec
		HoldoffMaxDelay: time.Duration(rand.Intn(500)) * time.Millisecond,
		Observer: &contour.RebuildMetricsObserver{
			Metrics:      metrics.NewMetrics(registry),
			NextObserver: dag.ComposeObservers(xdscache.ObserversOf(resources)...),
		},
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				FieldLogger: log,
				ConfiguredGateway: types.NamespacedName{
					Name:      "contour",
					Namespace: "projectcontour",
				},
			},
		},
	}

	eh.Builder.Processors = []dag.Processor{
		&dag.IngressProcessor{
			FieldLogger: log.WithField("context", "IngressProcessor"),
		},
		&dag.ExtensionServiceProcessor{
			FieldLogger: log.WithField("context", "ExtensionServiceProcessor"),
		},
		&dag.HTTPProxyProcessor{},
		&dag.GatewayAPIProcessor{
			FieldLogger: log.WithField("context", "GatewayAPIProcessor"),
		},
		&dag.ListenerProcessor{},
	}

	for _, opt := range opts {
		if opt, ok := opt.(func(*contour.EventHandler)); ok {
			opt(eh)
		}
	}

	// Make this event handler win the leader election.
	close(eh.IsLeader)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := xds.NewServer(registry)
	contour_xds_v3.RegisterServer(contour_xds_v3.NewContourServer(log, xdscache.ResourcesOf(resources)...), srv)

	var g workgroup.Group

	g.Add(func(stop <-chan struct{}) error {
		go func() {
			<-stop
			srv.GracefulStop()
		}()
		return srv.Serve(l) // srv now owns l and will close l before returning
	})
	g.Add(eh.Start())

	cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)

	rh := &resourceEventHandler{
		EventHandler:       eh,
		EndpointsHandler:   et,
		Sequence:           eh.Sequence,
		statusUpdateCacher: statusUpdateCacher,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error)
	go func() {
		done <- g.Run(ctx)
	}()

	return rh, &Contour{
			T:                 t,
			ClientConn:        cc,
			statusUpdateCache: statusUpdateCacher,
		}, func() {
			// close client connection
			cc.Close()

			// stop server
			cancel()

			<-done
		}
}

// resourceEventHandler composes a contour.EventHandler and a contour.EndpointsTranslator
// into a single ResourceEventHandler type. Its event handlers are *blocking* for non-Endpoints
// resources: they wait until the DAG has been rebuilt and observed, and the sequence counter
// has been incremented, before returning.
type resourceEventHandler struct {
	EventHandler     cache.ResourceEventHandler
	EndpointsHandler cache.ResourceEventHandler

	Sequence chan int

	statusUpdateCacher *k8s.StatusUpdateCacher
}

func (r *resourceEventHandler) OnAdd(obj interface{}) {
	if r.statusUpdateCacher.IsCacheable(obj) {
		r.statusUpdateCacher.OnAdd(obj)
	}

	switch obj.(type) {
	case *v1.Endpoints:
		r.EndpointsHandler.OnAdd(obj)
	default:
		r.EventHandler.OnAdd(obj)

		// Wait for the sequence counter to be incremented, which happens
		// after the DAG has been rebuilt and observed.
		<-r.Sequence
	}
}

func (r *resourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	// Ensure that tests don't sample stale status.
	if r.statusUpdateCacher.IsCacheable(oldObj) {
		r.statusUpdateCacher.OnDelete(oldObj)
	}

	if r.statusUpdateCacher.IsCacheable(newObj) {
		r.statusUpdateCacher.OnAdd(newObj)
	}

	switch newObj.(type) {
	case *v1.Endpoints:
		r.EndpointsHandler.OnUpdate(oldObj, newObj)
	default:
		r.EventHandler.OnUpdate(oldObj, newObj)

		// Wait for the sequence counter to be incremented, which happens
		// after the DAG has been rebuilt and observed.
		<-r.Sequence
	}
}

func (r *resourceEventHandler) OnDelete(obj interface{}) {
	// Delete this object from the status cache before we make
	// the deletion visible.
	if r.statusUpdateCacher.IsCacheable(obj) {
		r.statusUpdateCacher.OnDelete(obj)
	}

	switch obj.(type) {
	case *v1.Endpoints:
		r.EndpointsHandler.OnDelete(obj)
	default:
		r.EventHandler.OnDelete(obj)

		// Wait for the sequence counter to be incremented, which happens
		// after the DAG has been rebuilt and observed.
		<-r.Sequence
	}
}

// routeResources returns the given routes as a slice of any.Any
// resources, appropriately sorted.
func routeResources(t *testing.T, routes ...*envoy_route_v3.RouteConfiguration) []*any.Any {
	sort.Stable(sorter.For(routes))
	return resources(t, protobuf.AsMessages(routes)...)
}

func resources(t *testing.T, protos ...proto.Message) []*any.Any {
	t.Helper()
	anys := make([]*any.Any, 0, len(protos))
	for _, pb := range protos {
		anys = append(anys, protobuf.MustMarshalAny(pb))
	}
	return anys
}

type grpcStream interface {
	Send(*envoy_discovery_v3.DiscoveryRequest) error
	Recv() (*envoy_discovery_v3.DiscoveryResponse, error)
}

type StatusResult struct {
	*Contour

	Err  error
	Have *contour_api_v1.HTTPProxyStatus
}

// Equals asserts that the status result is not an error and matches
// the wanted status exactly.
func (s *StatusResult) Equals(want contour_api_v1.HTTPProxyStatus) *Contour {
	s.T.Helper()

	// We should never get an error fetching the status for an
	// object, so make it fatal if we do.
	if s.Err != nil {
		s.T.Fatalf(s.Err.Error())
	}

	assert.Equal(s.T, want, *s.Have)
	return s.Contour
}

// Like asserts that the status result is not an error and matches
// non-empty fields in the wanted status.
func (s *StatusResult) Like(want contour_api_v1.HTTPProxyStatus) *Contour {
	s.T.Helper()

	// We should never get an error fetching the status for an
	// object, so make it fatal if we do.
	if s.Err != nil {
		s.T.Fatalf(s.Err.Error())
	}

	if len(want.CurrentStatus) > 0 {
		assert.Equal(s.T,
			contour_api_v1.HTTPProxyStatus{CurrentStatus: want.CurrentStatus},
			contour_api_v1.HTTPProxyStatus{CurrentStatus: s.Have.CurrentStatus},
		)
	}

	if len(want.Description) > 0 {
		assert.Equal(s.T,
			contour_api_v1.HTTPProxyStatus{Description: want.Description},
			contour_api_v1.HTTPProxyStatus{Description: s.Have.Description},
		)
	}

	return s.Contour
}

// HasError asserts that there is an error on the Valid Condition in the proxy
// that matches the given values.
func (s *StatusResult) HasError(condType string, reason, message string) *Contour {
	assert.Equal(s.T, s.Have.CurrentStatus, string(status.ProxyStatusInvalid))
	assert.Equal(s.T, s.Have.Description, `At least one error present, see Errors for details`)
	validCond := s.Have.GetConditionFor(contour_api_v1.ValidConditionType)
	assert.NotNil(s.T, validCond)

	subCond, ok := validCond.GetError(condType)
	if !ok {
		s.T.Fatalf("Did not find error %s", condType)
	}
	assert.Equal(s.T, reason, subCond.Reason)
	assert.Equal(s.T, message, subCond.Message)

	return s.Contour
}

// IsValid asserts that the proxy's CurrentStatus field is equal to "valid".
func (s *StatusResult) IsValid() *Contour {
	s.T.Helper()

	assert.Equal(s.T, status.ProxyStatusValid, status.ProxyStatus(s.Have.CurrentStatus))

	return s.Contour
}

// IsInvalid asserts that the proxy's CurrentStatus field is equal to "invalid".
func (s *StatusResult) IsInvalid() *Contour {
	s.T.Helper()

	assert.Equal(s.T, status.ProxyStatusInvalid, status.ProxyStatus(s.Have.CurrentStatus))

	return s.Contour
}

type Contour struct {
	*grpc.ClientConn
	*testing.T

	statusUpdateCache *k8s.StatusUpdateCacher
}

// Status returns a StatusResult object that can be used to assert
// on object status fields.
func (c *Contour) Status(obj interface{}) *StatusResult {
	s, err := c.statusUpdateCache.GetStatus(obj)

	return &StatusResult{
		Contour: c,
		Err:     err,
		Have:    s,
	}
}

// NoStatus asserts that the given object did not get any status set.
func (c *Contour) NoStatus(obj interface{}) *Contour {
	if _, err := c.statusUpdateCache.GetStatus(obj); err == nil {
		c.T.Errorf("found cached object status, wanted no status")
	}

	return c
}

func (c *Contour) Request(typeurl string, names ...string) *Response {
	c.Helper()
	var st grpcStream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	switch typeurl {
	case secretType:
		sds := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(c.ClientConn)
		sts, err := sds.StreamSecrets(ctx)
		require.NoError(c, err)
		st = sts
	case routeType:
		rds := envoy_service_route_v3.NewRouteDiscoveryServiceClient(c.ClientConn)
		str, err := rds.StreamRoutes(ctx)
		require.NoError(c, err)
		st = str
	case clusterType:
		cds := envoy_service_cluster_v3.NewClusterDiscoveryServiceClient(c.ClientConn)
		stc, err := cds.StreamClusters(ctx)
		require.NoError(c, err)
		st = stc
	case listenerType:
		lds := envoy_service_listener_v3.NewListenerDiscoveryServiceClient(c.ClientConn)
		stl, err := lds.StreamListeners(ctx)
		require.NoError(c, err)
		st = stl
	case endpointType:
		eds := envoy_service_endpoint_v3.NewEndpointDiscoveryServiceClient(c.ClientConn)
		ste, err := eds.StreamEndpoints(ctx)
		require.NoError(c, err)
		st = ste
	default:
		c.Fatal("unknown typeURL:", typeurl)
	}
	resp := c.sendRequest(st, &envoy_discovery_v3.DiscoveryRequest{
		TypeUrl:       typeurl,
		ResourceNames: names,
	})
	return &Response{
		Contour:           c,
		DiscoveryResponse: resp,
	}
}

func (c *Contour) sendRequest(stream grpcStream, req *envoy_discovery_v3.DiscoveryRequest) *envoy_discovery_v3.DiscoveryResponse {
	err := stream.Send(req)
	require.NoError(c, err)
	resp, err := stream.Recv()
	require.NoError(c, err)
	return resp
}

type Response struct {
	*Contour
	*envoy_discovery_v3.DiscoveryResponse
}

// Equals tests that the response retrieved from Contour is equal to the supplied value.
// TODO(youngnick) This function really should be copied to an `EqualResources` function.
func (r *Response) Equals(want *envoy_discovery_v3.DiscoveryResponse) *Contour {
	r.Helper()

	protobuf.RequireEqual(r.T, want.Resources, r.DiscoveryResponse.Resources)

	return r.Contour
}
