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
	"path"
	"testing"
	"time"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	ratelimit_config_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	ratelimit_filter_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestListenerCacheContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_listener_v3.Listener
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
			want: []proto.Message{
				&envoy_listener_v3.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
					SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var lc ListenerCache
			lc.Update(tc.contents)
			got := lc.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestListenerCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_listener_v3.Listener
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
			query: []string{ENVOY_HTTP_LISTENER},
			want: []proto.Message{
				&envoy_listener_v3.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
					SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
				},
			},
		},
		"partial match": {
			contents: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
			query: []string{ENVOY_HTTP_LISTENER, "stats-listener"},
			want: []proto.Message{
				&envoy_listener_v3.Listener{
					Name:          ENVOY_HTTP_LISTENER,
					Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
					FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
					SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
				},
			},
		},
		"no match": {
			contents: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
			query: []string{"stats-listener"},
			want:  nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var lc ListenerCache
			lc.Update(tc.contents)
			got := lc.Query(tc.query)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestListenerVisit(t *testing.T) {
	httpsFilterFor := func(vhost string) *envoy_listener_v3.Filter {
		return envoy_v3.HTTPConnectionManagerBuilder().
			AddFilter(envoy_v3.FilterMisdirectedRequests(vhost)).
			DefaultFilters().
			MetricsPrefix(ENVOY_HTTPS_LISTENER).
			RouteConfigName(path.Join("https", vhost)).
			AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
			Get()
	}

	fallbackCertFilter := envoy_v3.HTTPConnectionManagerBuilder().
		DefaultFilters().
		MetricsPrefix(ENVOY_HTTPS_LISTENER).
		RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
		Get()

	tests := map[string]struct {
		ListenerConfig
		fallbackCertificate *types.NamespacedName
		objs                []interface{}
		want                map[string]*envoy_listener_v3.Listener
	}{
		"nothing": {
			objs: nil,
			want: map[string]*envoy_listener_v3.Listener{},
		},
		"one http only ingress": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"one http only httpproxy": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"simple ingress with secret": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"multiple tls ingress with secrets should be sorted": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sortedsecond",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"sortedsecond.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "sortedsecond.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sortedfirst",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"sortedfirst.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "sortedfirst.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"sortedfirst.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("sortedfirst.example.com")),
				}, {
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"sortedsecond.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("sortedsecond.example.com")),
				}},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"simple ingress with missing secret": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "missing",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"simple httpproxy with secret": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"ingress with allow-http: false": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernetes.io/ingress.allow-http": "false",
						},
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: backend("kuard", 8080),
					},
				},
			},
			want: map[string]*envoy_listener_v3.Listener{},
		},
		"simple tls ingress with allow-http:false": {
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernetes.io/ingress.allow-http": "false",
						},
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"www.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"http listener on non default port": { // issue 72
			ListenerConfig: ListenerConfig{
				HTTPListeners: map[string]Listener{
					ENVOY_HTTP_LISTENER: {
						Name:    ENVOY_HTTP_LISTENER,
						Address: "127.0.0.100",
						Port:    9100,
					},
				},
				HTTPSListeners: map[string]Listener{
					ENVOY_HTTPS_LISTENER: {
						Name:    ENVOY_HTTPS_LISTENER,
						Address: "127.0.0.200",
						Port:    9200,
					},
				},
			},
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("127.0.0.100", 9100),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("127.0.0.200", 9200),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"use proxy proto": {
			ListenerConfig: ListenerConfig{
				UseProxyProto: true,
			},
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.ProxyProtocol(),
				),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.ProxyProtocol(),
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("whatever.example.com")),
				}},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"--envoy-http-access-log": {
			ListenerConfig: ListenerConfig{
				HTTPAccessLog:  "/tmp/http_access.log",
				HTTPSAccessLog: "/tmp/https_access.log",
			},
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress(DEFAULT_HTTP_LISTENER_ADDRESS, DEFAULT_HTTP_LISTENER_PORT),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy("/tmp/http_access.log", "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress(DEFAULT_HTTPS_LISTENER_ADDRESS, DEFAULT_HTTPS_LISTENER_PORT),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("whatever.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "whatever.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy("/tmp/https_access.log", "", nil)).
						Get()),
				}},
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.3",
			},
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config overridden by annotation": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.3",
			},
			objs: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
						Annotations: map[string]string{
							"projectcontour.io/tls-minimum-protocol-version": "1.2",
						},
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     8080,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"whatever.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy_v3.Filters(httpsFilterFor("whatever.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-min-protocol-version from config overridden by httpproxy": {
			ListenerConfig: ListenerConfig{
				MinimumTLSVersion: "1.3",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:             "secret",
								MinimumProtocolVersion: "1.2",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_3, nil, "h2", "http/1.1"), // note, cannot downgrade from the configured version
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"tls-cipher-suites from config": {
			ListenerConfig: ListenerConfig{
				CipherSuites: []string{
					"ECDHE-ECDSA-AES256-GCM-SHA384",
					"ECDHE-RSA-AES256-GCM-SHA384",
				},
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, []string{"ECDHE-ECDSA-AES256-GCM-SHA384", "ECDHE-RSA-AES256-GCM-SHA384"}, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate and with request timeout set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				RequestTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						RequestTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						RequestTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						RequestTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate and with connection idle timeout set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				ConnectionIdleTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate and with stream idle timeout set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				StreamIdleTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate and with delayed close timeout set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				DelayedCloseTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate and with max connection duration set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				MaxConnectionDuration: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate and with connection shutdown grace period set": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				ConnectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}, {
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(fallbackCertFilter),
					Name:            "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"multiple httpproxies with fallback certificate": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple2",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.another.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{
					{
						FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
							ServerNames: []string{"www.another.com"},
						},
						TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
						Filters:         envoy_v3.Filters(httpsFilterFor("www.another.com")),
					},
					{
						FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
							ServerNames: []string{"www.example.com"},
						},
						TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
						Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
					},
					{
						FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
							TransportProtocol: "tls",
						},
						TransportSocket: transportSocket("fallbacksecret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
						Filters:         envoy_v3.Filters(fallbackCertFilter),
						Name:            "fallback-certificate",
					}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with fallback certificate - no cert passed": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "",
				Namespace: "",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(),
		},
		"httpproxy with fallback certificate - cert passed but vhost not enabled": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbackcert",
				Namespace: "default",
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: false,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:          ENVOY_HTTP_LISTENER,
				Address:       envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains:  envoy_v3.FilterChains(envoy_v3.HTTPConnectionManager(ENVOY_HTTP_LISTENER, envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil), 0, 0)),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters:         envoy_v3.Filters(httpsFilterFor("www.example.com")),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with connection idle timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				ConnectionIdleTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with stream idle timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				StreamIdleTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with max connection duration set in visitor config": {
			ListenerConfig: ListenerConfig{
				MaxConnectionDuration: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with delayed close timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				DelayedCloseTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with connection shutdown grace period set in visitor config": {
			ListenerConfig: ListenerConfig{
				ConnectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with connection idle timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				ConnectionIdleTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
					DefaultFilters().
					ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						ConnectionIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with allow_chunked_length set in visitor config": {
			ListenerConfig: ListenerConfig{
				AllowChunkedLength: true,
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						AllowChunkedLength(true).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpproxy with XffNumTrustedHops set in visitor config": {
			ListenerConfig: ListenerConfig{
				XffNumTrustedHops: 1,
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						MetricsPrefix(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						NumTrustedHops(1).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with stream idle timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				StreamIdleTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
					DefaultFilters().
					StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						StreamIdleTimeout(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with max connection duration set in visitor config": {
			ListenerConfig: ListenerConfig{
				MaxConnectionDuration: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
					DefaultFilters().
					MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						MaxConnectionDuration(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with delayed close timeout set in visitor config": {
			ListenerConfig: ListenerConfig{
				DelayedCloseTimeout: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
					DefaultFilters().
					DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DelayedCloseTimeout(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"httpsproxy with secret with connection shutdown grace period set in visitor config": {
			ListenerConfig: ListenerConfig{
				ConnectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
					DefaultFilters().
					ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
					Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						ConnectionShutdownGracePeriod(timeout.DurationSetting(90 * time.Second)).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"insecure httpproxy with rate limit config": {
			ListenerConfig: ListenerConfig{
				RateLimitConfig: &RateLimitConfig{
					ExtensionService:        types.NamespacedName{Namespace: "projectcontour", Name: "ratelimit"},
					Domain:                  "contour",
					Timeout:                 timeout.DurationSetting(7 * time.Second),
					FailOpen:                false,
					EnableXRateLimitHeaders: true,
				},
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
					RouteConfigName("ingress_http").
					MetricsPrefix("ingress_http").
					AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil)).
					DefaultFilters().
					AddFilter(&http.HttpFilter{
						Name: wellknown.HTTPRateLimit,
						ConfigType: &http.HttpFilter_TypedConfig{
							TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
								Domain:          "contour",
								FailureModeDeny: true,
								Timeout:         protobuf.Duration(7 * time.Second),
								RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
									GrpcService: &envoy_core_v3.GrpcService{
										TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
												ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
											},
										},
									},
									TransportApiVersion: envoy_core_v3.ApiVersion_V3,
								},
								EnableXRatelimitHeaders: ratelimit_filter_v3.RateLimit_DRAFT_VERSION_03,
							}),
						},
					}).Get()),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"secure httpproxy with rate limit config": {
			ListenerConfig: ListenerConfig{
				RateLimitConfig: &RateLimitConfig{
					ExtensionService:        types.NamespacedName{Namespace: "projectcontour", Name: "ratelimit"},
					Domain:                  "contour",
					Timeout:                 timeout.DurationSetting(7 * time.Second),
					FailOpen:                false,
					EnableXRateLimitHeaders: true,
				},
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(envoy_v3.HTTPConnectionManagerBuilder().
					RouteConfigName(ENVOY_HTTP_LISTENER).
					MetricsPrefix(ENVOY_HTTP_LISTENER).
					AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
					DefaultFilters().
					AddFilter(&http.HttpFilter{
						Name: wellknown.HTTPRateLimit,
						ConfigType: &http.HttpFilter_TypedConfig{
							TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
								Domain:          "contour",
								FailureModeDeny: true,
								Timeout:         protobuf.Duration(7 * time.Second),
								RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
									GrpcService: &envoy_core_v3.GrpcService{
										TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
												ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
											},
										},
									},
									TransportApiVersion: envoy_core_v3.ApiVersion_V3,
								},
								EnableXRatelimitHeaders: ratelimit_filter_v3.RateLimit_DRAFT_VERSION_03,
							}),
						},
					}).
					Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						AddFilter(&http.HttpFilter{
							Name: wellknown.HTTPRateLimit,
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
									Domain:          "contour",
									FailureModeDeny: true,
									Timeout:         protobuf.Duration(7 * time.Second),
									RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
										GrpcService: &envoy_core_v3.GrpcService{
											TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
												EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
													ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
												},
											},
										},
										TransportApiVersion: envoy_core_v3.ApiVersion_V3,
									},
									EnableXRatelimitHeaders: ratelimit_filter_v3.RateLimit_DRAFT_VERSION_03,
								}),
							},
						}).
						Get()),
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
		"secure httpproxy using fallback certificate with rate limit config": {
			fallbackCertificate: &types.NamespacedName{
				Name:      "fallbacksecret",
				Namespace: "default",
			},
			ListenerConfig: ListenerConfig{
				RateLimitConfig: &RateLimitConfig{
					ExtensionService:        types.NamespacedName{Namespace: "projectcontour", Name: "ratelimit"},
					Domain:                  "contour",
					Timeout:                 timeout.DurationSetting(7 * time.Second),
					FailOpen:                false,
					EnableXRateLimitHeaders: true,
				},
			},
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                "secret",
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{
							{
								Services: []contour_api_v1.Service{
									{
										Name: "backend",
										Port: 80,
									},
								},
							},
						},
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbacksecret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
				},
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:     "http",
							Protocol: "TCP",
							Port:     80,
						}},
					},
				},
			},
			want: listenermap(&envoy_listener_v3.Listener{
				Name:    ENVOY_HTTP_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
				FilterChains: envoy_v3.FilterChains(
					envoy_v3.HTTPConnectionManagerBuilder().
						RouteConfigName(ENVOY_HTTP_LISTENER).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						DefaultFilters().
						AddFilter(&http.HttpFilter{
							Name: wellknown.HTTPRateLimit,
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
									Domain:          "contour",
									FailureModeDeny: true,
									Timeout:         protobuf.Duration(7 * time.Second),
									RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
										GrpcService: &envoy_core_v3.GrpcService{
											TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
												EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
													ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
												},
											},
										},
										TransportApiVersion: envoy_core_v3.ApiVersion_V3,
									},
									EnableXRatelimitHeaders: ratelimit_filter_v3.RateLimit_DRAFT_VERSION_03,
								}),
							},
						}).
						Get(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}, &envoy_listener_v3.Listener{
				Name:    ENVOY_HTTPS_LISTENER,
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				FilterChains: []*envoy_listener_v3.FilterChain{{
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
					TransportSocket: transportSocket("secret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						AddFilter(envoy_v3.FilterMisdirectedRequests("www.example.com")).
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(path.Join("https", "www.example.com")).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						AddFilter(&http.HttpFilter{
							Name: wellknown.HTTPRateLimit,
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
									Domain:          "contour",
									FailureModeDeny: true,
									Timeout:         protobuf.Duration(7 * time.Second),
									RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
										GrpcService: &envoy_core_v3.GrpcService{
											TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
												EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
													ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
												},
											},
										},
										TransportApiVersion: envoy_core_v3.ApiVersion_V3,
									},
									EnableXRatelimitHeaders: ratelimit_filter_v3.RateLimit_DRAFT_VERSION_03,
								}),
							},
						}).
						Get(),
					),
				}, {
					FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
						TransportProtocol: "tls",
					},
					TransportSocket: transportSocket("fallbacksecret", envoy_tls_v3.TlsParameters_TLSv1_2, nil, "h2", "http/1.1"),
					Filters: envoy_v3.Filters(envoy_v3.HTTPConnectionManagerBuilder().
						DefaultFilters().
						MetricsPrefix(ENVOY_HTTPS_LISTENER).
						RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
						AccessLoggers(envoy_v3.FileAccessLogEnvoy(DEFAULT_HTTP_ACCESS_LOG, "", nil)).
						AddFilter(&http.HttpFilter{
							Name: wellknown.HTTPRateLimit,
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
									Domain:          "contour",
									FailureModeDeny: true,
									Timeout:         protobuf.Duration(7 * time.Second),
									RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
										GrpcService: &envoy_core_v3.GrpcService{
											TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
												EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
													ClusterName: dag.ExtensionClusterName(k8s.NamespacedNameFrom("projectcontour/ratelimit")),
												},
											},
										},
										TransportApiVersion: envoy_core_v3.ApiVersion_V3,
									},
									EnableXRatelimitHeaders: ratelimit_filter_v3.RateLimit_DRAFT_VERSION_03,
								}),
							},
						}).
						Get(),
					),
					Name: "fallback-certificate",
				}},
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := buildDAGFallback(t, tc.fallbackCertificate, tc.objs...)
			got := visitListeners(root, &tc.ListenerConfig)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func transportSocket(secretname string, tlsMinProtoVersion envoy_tls_v3.TlsParameters_TlsProtocol, cipherSuites []string, alpnprotos ...string) *envoy_core_v3.TransportSocket {
	secret := &dag.Secret{
		Object: &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretname,
				Namespace: "default",
			},
			Type: v1.SecretTypeTLS,
			Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
		},
	}
	return envoy_v3.DownstreamTLSTransportSocket(
		envoy_v3.DownstreamTLSContext(secret, tlsMinProtoVersion, cipherSuites, nil, alpnprotos...),
	)
}

func listenermap(listeners ...*envoy_listener_v3.Listener) map[string]*envoy_listener_v3.Listener {
	m := make(map[string]*envoy_listener_v3.Listener)
	for _, l := range listeners {
		m[l.Name] = l
	}
	return m
}
