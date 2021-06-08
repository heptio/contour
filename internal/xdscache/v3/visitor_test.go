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
	"testing"

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestVisitClusters(t *testing.T) {
	sec := new(dag.Secret)
	tests := map[string]struct {
		root dag.Vertex
		want map[string]*envoy_cluster_v3.Cluster
	}{
		"TCPService forward": {
			root: &dag.Listener{
				Port: 443,
				VirtualHosts: virtualhosts(
					&dag.SecureVirtualHost{
						VirtualHost: dag.VirtualHost{
							Name: "www.example.com",
						},
						TCPProxy: &dag.TCPProxy{
							Clusters: []*dag.Cluster{{
								Upstream: &dag.Service{
									Weighted: dag.WeightedService{
										Weight:           1,
										ServiceName:      "example",
										ServiceNamespace: "default",
										ServicePort: v1.ServicePort{
											Protocol:   "TCP",
											Port:       443,
											TargetPort: intstr.FromInt(8443),
										},
									},
								},
							}},
						},
						Secrets: map[string]*dag.Secret{sec.Name(): sec},
					},
				),
			},
			want: clustermap(
				&envoy_cluster_v3.Cluster{
					Name:                 "default/example/443/da39a3ee5e",
					AltStatName:          "default_example_443",
					ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
					EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
						EdsConfig:   envoy_v3.ConfigSource("contour"),
						ServiceName: "default/example",
					},
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := visitClusters(tc.root)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestVisitListeners(t *testing.T) {
	p1 := &dag.TCPProxy{
		Clusters: []*dag.Cluster{{
			Upstream: &dag.Service{
				Weighted: dag.WeightedService{
					Weight:           1,
					ServiceName:      "example",
					ServiceNamespace: "default",
					ServicePort: v1.ServicePort{
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					},
				},
			},
		}},
	}

	tests := map[string]struct {
		root dag.Vertex
		want map[string]*envoy_listener_v3.Listener
	}{
		"TCPService forward": {
			root: &dag.Listener{
				Port: 443,
				VirtualHosts: virtualhosts(
					&dag.SecureVirtualHost{
						VirtualHost: dag.VirtualHost{
							Name:         "tcpproxy.example.com",
							ListenerName: "ingress_https",
						},
						TCPProxy: p1,
						Secrets: map[string]*dag.Secret{"secret": {
							Object: &v1.Secret{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "secret",
									Namespace: "default",
								},
								Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
							},
						}},
						MinTLSVersion: "1.2",
					},
				),
			},
			want: listenermap(
				&envoy_listener_v3.Listener{
					Name:    ENVOY_HTTPS_LISTENER,
					Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
					FilterChains: []*envoy_listener_v3.FilterChain{{
						FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
							ServerNames: []string{"tcpproxy.example.com"},
						},
						TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil),
						Filters:         envoy_v3.Filters(envoy_v3.TCPProxy(ENVOY_HTTPS_LISTENER, p1, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTPS_ACCESS_LOG))),
					}},
					ListenerFilters: envoy_v3.ListenerFilters(
						envoy_v3.TLSInspector(),
					),
					SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := visitListeners(tc.root, new(ListenerConfig))
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestVisitSecrets(t *testing.T) {
	tests := map[string]struct {
		root dag.Vertex
		want map[string]*envoy_tls_v3.Secret
	}{
		"TCPService forward": {
			root: &dag.Listener{
				Port: 443,
				VirtualHosts: virtualhosts(
					&dag.SecureVirtualHost{
						VirtualHost: dag.VirtualHost{
							Name: "www.example.com",
						},
						TCPProxy: &dag.TCPProxy{
							Clusters: []*dag.Cluster{{
								Upstream: &dag.Service{
									Weighted: dag.WeightedService{
										Weight:           1,
										ServiceName:      "example",
										ServiceNamespace: "default",
										ServicePort: v1.ServicePort{
											Protocol:   "TCP",
											Port:       443,
											TargetPort: intstr.FromInt(8443),
										},
									},
								},
							}},
						},
						Secrets: map[string]*dag.Secret{"secret": {
							Object: &v1.Secret{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "secret",
									Namespace: "default",
								},
								Data: secretdata("certificate", "key"),
							},
						}},
					},
				),
			},
			want: secretmap(&envoy_tls_v3.Secret{
				Name: "default/secret/735ad571c1",
				Type: &envoy_tls_v3.Secret_TlsCertificate{
					TlsCertificate: &envoy_tls_v3.TlsCertificate{
						PrivateKey: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_InlineBytes{
								InlineBytes: []byte("key"),
							},
						},
						CertificateChain: &envoy_core_v3.DataSource{
							Specifier: &envoy_core_v3.DataSource_InlineBytes{
								InlineBytes: []byte("certificate"),
							},
						},
					},
				},
			}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := visitSecrets(tc.root)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func virtualhosts(vx ...dag.Vertex) []dag.Vertex {
	return vx
}
