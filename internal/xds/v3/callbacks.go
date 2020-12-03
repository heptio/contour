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

import (
	"fmt"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_server_v3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/sirupsen/logrus"
)

func NewCallbacks(log *logrus.Logger) envoy_server_v3.Callbacks {
	return &envoy_server_v3.CallbackFuncs{
		StreamRequestFunc: func(streamID int64, req *envoy_service_discovery_v3.DiscoveryRequest) error {
			logDiscoveryRequestDetails(log, req)
			return nil
		},
	}
}

func logDiscoveryRequestDetails(l *logrus.Logger, req *envoy_service_discovery_v3.DiscoveryRequest) {
	log := l.WithField("version_info", req.VersionInfo).WithField("response_nonce", req.ResponseNonce)
	if req.Node != nil {
		log = log.WithField("node_id", req.Node.Id).WithField("node_version", fmt.Sprintf("v%d.%d.%d", req.Node.GetUserAgentBuildVersion().Version.MajorNumber, req.Node.GetUserAgentBuildVersion().Version.MinorNumber, req.Node.GetUserAgentBuildVersion().Version.Patch))
	}

	if status := req.ErrorDetail; status != nil {
		// if Envoy rejected the last update log the details here.
		// TODO(dfc) issue 1176: handle xDS ACK/NACK
		log.WithField("code", status.Code).Error(status.Message)
	}

	log = log.WithField("resource_names", req.ResourceNames).WithField("type_url", req.GetTypeUrl())
	log.Info("handling v3 xDS resource request")
}
