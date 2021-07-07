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

// envoy helpers

import (
	"path"
	"time"

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_config_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_tcp_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_extensions_upstream_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultCluster returns a copy of the default Cluster, with each
// Cluster given in the parameter slice merged on top. This makes it
// relatively fluent to compose Clusters by tweaking a few fields.
func DefaultCluster(clusters ...*envoy_cluster_v3.Cluster) *envoy_cluster_v3.Cluster {
	// NOTE: Keep this in sync with envoy.defaultCluster().
	defaults := &envoy_cluster_v3.Cluster{
		ConnectTimeout: protobuf.Duration(250 * time.Millisecond),
		LbPolicy:       envoy_cluster_v3.Cluster_ROUND_ROBIN,
		CommonLbConfig: envoy_v3.ClusterCommonLBConfig(),
	}

	for _, c := range clusters {
		proto.Merge(defaults, c)
	}

	return defaults
}

func clusterWithHealthCheck(name, servicename, statName, healthCheckPath string, drainConnOnHostRemoval bool) *envoy_cluster_v3.Cluster {
	c := cluster(name, servicename, statName)
	c.HealthChecks = []*envoy_core_v3.HealthCheck{{
		Timeout:            protobuf.Duration(2 * time.Second),
		Interval:           protobuf.Duration(10 * time.Second),
		UnhealthyThreshold: protobuf.UInt32(3),
		HealthyThreshold:   protobuf.UInt32(2),
		HealthChecker: &envoy_core_v3.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoy_core_v3.HealthCheck_HttpHealthCheck{
				Host: "contour-envoy-healthcheck",
				Path: healthCheckPath,
			},
		},
	}}
	c.IgnoreHealthOnHostRemoval = drainConnOnHostRemoval
	return c
}

func externalNameCluster(name, servicename, statName, externalName string, port int) *envoy_cluster_v3.Cluster {
	return DefaultCluster(&envoy_cluster_v3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_STRICT_DNS),
		AltStatName:          statName,
		LoadAssignment: &envoy_endpoint_v3.ClusterLoadAssignment{
			ClusterName: servicename,
			Endpoints: envoy_v3.Endpoints(
				envoy_v3.SocketAddress(externalName, port),
			),
		},
	})
}

func routeCluster(cluster string, opts ...func(*envoy_route_v3.Route_Route)) *envoy_route_v3.Route_Route {
	r := &envoy_route_v3.Route_Route{
		Route: &envoy_route_v3.RouteAction{
			ClusterSpecifier: &envoy_route_v3.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}

	for _, o := range opts {
		o(r)
	}

	return r
}

func routePrefix(prefix string, headers ...dag.HeaderMatchCondition) *envoy_route_v3.RouteMatch {
	return envoy_v3.RouteMatch(&dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{
			Prefix: prefix,
		},
		HeaderMatchConditions: headers,
	})
}

func routeHostRewrite(cluster, newHostName string) *envoy_route_v3.Route_Route {
	return &envoy_route_v3.Route_Route{
		Route: &envoy_route_v3.RouteAction{
			ClusterSpecifier:     &envoy_route_v3.RouteAction_Cluster{Cluster: cluster},
			HostRewriteSpecifier: &envoy_route_v3.RouteAction_HostRewriteLiteral{HostRewriteLiteral: newHostName},
		},
	}
}

func upgradeHTTPS(match *envoy_route_v3.RouteMatch) *envoy_route_v3.Route {
	return &envoy_route_v3.Route{
		Match:  match,
		Action: envoy_v3.UpgradeHTTPS(),
	}
}

func cluster(name, servicename, statName string) *envoy_cluster_v3.Cluster {
	return DefaultCluster(&envoy_cluster_v3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_EDS),
		AltStatName:          statName,
		EdsClusterConfig: &envoy_cluster_v3.Cluster_EdsClusterConfig{
			EdsConfig:   envoy_v3.ConfigSource("contour"),
			ServiceName: servicename,
		},
	})
}

func tlsCluster(c *envoy_cluster_v3.Cluster, ca []byte, subjectName string, sni string, clientSecret *v1.Secret, alpnProtocols ...string) *envoy_cluster_v3.Cluster {
	var secret *dag.Secret
	if clientSecret != nil {
		secret = &dag.Secret{Object: clientSecret}
	}

	c.TransportSocket = envoy_v3.UpstreamTLSTransportSocket(
		envoy_v3.UpstreamTLSContext(
			&dag.PeerValidationContext{
				CACertificate: &dag.Secret{Object: &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Type: "kubernetes.io/tls",
					Data: map[string][]byte{dag.CACertificateKey: ca},
				}},
				SubjectName: subjectName},
			sni,
			secret,
			alpnProtocols...,
		),
	)
	return c
}

func tlsClusterWithoutValidation(c *envoy_cluster_v3.Cluster, sni string, clientSecret *v1.Secret, alpnProtocols ...string) *envoy_cluster_v3.Cluster {
	var secret *dag.Secret
	if clientSecret != nil {
		secret = &dag.Secret{Object: clientSecret}
	}

	c.TransportSocket = envoy_v3.UpstreamTLSTransportSocket(
		envoy_v3.UpstreamTLSContext(
			nil,
			sni,
			secret,
			alpnProtocols...,
		),
	)
	return c
}

func h2cCluster(c *envoy_cluster_v3.Cluster) *envoy_cluster_v3.Cluster {
	c.TypedExtensionProtocolOptions = map[string]*any.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protobuf.MustMarshalAny(
			&envoy_extensions_upstream_http_v3.HttpProtocolOptions{
				UpstreamProtocolOptions: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
					ExplicitHttpConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
						ProtocolConfig: &envoy_extensions_upstream_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
					},
				},
			}),
	}
	return c
}

func withResponseTimeout(route *envoy_route_v3.Route_Route, timeout time.Duration) *envoy_route_v3.Route_Route {
	route.Route.Timeout = protobuf.Duration(timeout)
	return route
}

func withIdleTimeout(route *envoy_route_v3.Route_Route, timeout time.Duration) *envoy_route_v3.Route_Route {
	route.Route.IdleTimeout = protobuf.Duration(timeout)
	return route
}

func withMirrorPolicy(route *envoy_route_v3.Route_Route, mirror string) *envoy_route_v3.Route_Route {
	route.Route.RequestMirrorPolicies = []*envoy_route_v3.RouteAction_RequestMirrorPolicy{{
		Cluster: mirror,
	}}
	return route
}

func withPrefixRewrite(route *envoy_route_v3.Route_Route, replacement string) *envoy_route_v3.Route_Route {
	route.Route.PrefixRewrite = replacement
	return route
}

func withRetryPolicy(route *envoy_route_v3.Route_Route, retryOn string, numRetries uint32, perTryTimeout time.Duration) *envoy_route_v3.Route_Route {
	route.Route.RetryPolicy = &envoy_route_v3.RetryPolicy{
		RetryOn: retryOn,
	}
	if numRetries > 0 {
		route.Route.RetryPolicy.NumRetries = protobuf.UInt32(numRetries)
	}
	if perTryTimeout > 0 {
		route.Route.RetryPolicy.PerTryTimeout = protobuf.Duration(perTryTimeout)
	}
	return route
}

func withWebsocket(route *envoy_route_v3.Route_Route) *envoy_route_v3.Route_Route {
	route.Route.UpgradeConfigs = append(route.Route.UpgradeConfigs,
		&envoy_route_v3.RouteAction_UpgradeConfig{
			UpgradeType: "websocket",
		},
	)
	return route
}

func withSessionAffinity(route *envoy_route_v3.Route_Route) *envoy_route_v3.Route_Route {
	route.Route.HashPolicy = append(route.Route.HashPolicy, &envoy_route_v3.RouteAction_HashPolicy{
		PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_Cookie_{
			Cookie: &envoy_route_v3.RouteAction_HashPolicy_Cookie{
				Name: "X-Contour-Session-Affinity",
				Ttl:  protobuf.Duration(0),
				Path: "/",
			},
		},
	})
	return route
}

type headerTerminalPair struct {
	headerName string
	terminal   bool
}

func withRequestHashPolicyHeader(route *envoy_route_v3.Route_Route, headerOptions ...headerTerminalPair) *envoy_route_v3.Route_Route {
	for _, htp := range headerOptions {
		route.Route.HashPolicy = append(route.Route.HashPolicy, &envoy_route_v3.RouteAction_HashPolicy{
			Terminal: htp.terminal,
			PolicySpecifier: &envoy_route_v3.RouteAction_HashPolicy_Header_{
				Header: &envoy_route_v3.RouteAction_HashPolicy_Header{
					HeaderName: htp.headerName,
				},
			},
		})
	}
	return route
}

func withRedirect() *envoy_route_v3.Route_Redirect {
	return &envoy_route_v3.Route_Redirect{
		Redirect: &envoy_route_v3.RedirectAction{
			SchemeRewriteSpecifier: &envoy_route_v3.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}
}

func withFilterConfig(name string, message proto.Message) map[string]*any.Any {
	return map[string]*any.Any{
		name: protobuf.MustMarshalAny(message),
	}
}

type weightedCluster struct {
	name   string
	weight uint32
}

func routeWeightedCluster(clusters ...weightedCluster) *envoy_route_v3.Route_Route {
	return &envoy_route_v3.Route_Route{
		Route: &envoy_route_v3.RouteAction{
			ClusterSpecifier: &envoy_route_v3.RouteAction_WeightedClusters{
				WeightedClusters: weightedClusters(clusters),
			},
		},
	}
}

func weightedClusters(clusters []weightedCluster) *envoy_route_v3.WeightedCluster {
	var wc envoy_route_v3.WeightedCluster
	var total uint32
	for _, c := range clusters {
		total += c.weight
		wc.Clusters = append(wc.Clusters, &envoy_route_v3.WeightedCluster_ClusterWeight{
			Name:   c.name,
			Weight: protobuf.UInt32(c.weight),
		})
	}
	wc.TotalWeight = protobuf.UInt32(total)
	return &wc
}

// appendFilterChains is a helper to turn variadic FilterChain arguments into the corresponding  slice.
func appendFilterChains(chains ...*envoy_listener_v3.FilterChain) []*envoy_listener_v3.FilterChain {
	return chains
}

// filterchaintls returns a FilterChain wrapping the given virtual host.
func filterchaintls(domain string, secret *v1.Secret, filter *envoy_listener_v3.Filter, peerValidationContext *dag.PeerValidationContext, alpn ...string) *envoy_listener_v3.FilterChain {
	return envoy_v3.FilterChainTLS(
		domain,
		envoy_v3.DownstreamTLSContext(
			&dag.Secret{Object: secret},
			envoy_tls_v3.TlsParameters_TLSv1_2,
			nil,
			peerValidationContext,
			alpn...),
		envoy_v3.Filters(filter),
	)
}

// filterchaintlsfallback returns a FilterChain for the given TLS fallback certificate.
func filterchaintlsfallback(fallbackSecret *v1.Secret, peerValidationContext *dag.PeerValidationContext, alpn ...string) *envoy_listener_v3.FilterChain {
	return envoy_v3.FilterChainTLSFallback(
		envoy_v3.DownstreamTLSContext(
			&dag.Secret{Object: fallbackSecret},
			envoy_tls_v3.TlsParameters_TLSv1_2,
			nil,
			peerValidationContext,
			alpn...),
		envoy_v3.Filters(
			envoy_v3.HTTPConnectionManagerBuilder().
				DefaultFilters().
				RouteConfigName(xdscache_v3.ENVOY_FALLBACK_ROUTECONFIG).
				MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
				AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil)).
				Get(),
		),
	)
}

func httpsFilterFor(vhost string) *envoy_listener_v3.Filter {
	return envoy_v3.HTTPConnectionManagerBuilder().
		AddFilter(envoy_v3.FilterMisdirectedRequests(vhost)).
		DefaultFilters().
		RouteConfigName(path.Join("https", vhost)).
		MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil)).
		Get()
}

// authzFilterFor does the same as httpsFilterFor but inserts a
// `ext_authz` filter with the specified configuration into the
// filter chain.
func authzFilterFor(
	vhost string,
	authz *envoy_config_filter_http_ext_authz_v3.ExtAuthz,
) *envoy_listener_v3.Filter {
	return envoy_v3.HTTPConnectionManagerBuilder().
		AddFilter(envoy_v3.FilterMisdirectedRequests(vhost)).
		DefaultFilters().
		AddFilter(&http.HttpFilter{
			Name: "envoy.filters.http.ext_authz",
			ConfigType: &http.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(authz),
			},
		}).
		RouteConfigName(path.Join("https", vhost)).
		MetricsPrefix(xdscache_v3.ENVOY_HTTPS_LISTENER).
		AccessLoggers(envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil)).
		Get()
}

func tcpproxy(statPrefix, cluster string) *envoy_listener_v3.Filter {
	return &envoy_listener_v3.Filter{
		Name: wellknown.TCPProxy,
		ConfigType: &envoy_listener_v3.Filter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_tcp_proxy_v3.TcpProxy{
				StatPrefix: statPrefix,
				ClusterSpecifier: &envoy_tcp_proxy_v3.TcpProxy_Cluster{
					Cluster: cluster,
				},
				AccessLog:   envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil),
				IdleTimeout: protobuf.Duration(9001 * time.Second),
			}),
		},
	}
}

func staticListener() *envoy_listener_v3.Listener {
	return envoy_v3.StatsListener("0.0.0.0", 8002)
}

func defaultHTTPListener() *envoy_listener_v3.Listener {
	return &envoy_listener_v3.Listener{
		Name:    "ingress_http",
		Address: envoy_v3.SocketAddress("0.0.0.0", 8080),
		FilterChains: envoy_v3.FilterChains(
			envoy_v3.HTTPConnectionManager("ingress_http", envoy_v3.FileAccessLogEnvoy("/dev/stdout", "", nil), 0, 0),
		),
		SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
	}
}
