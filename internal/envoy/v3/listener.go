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
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	accesslog "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_compressor_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	envoy_config_filter_http_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_config_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	lua "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	envoy_extensions_filters_http_router_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"github.com/projectcontour/contour/internal/timeout"
)

type HTTPVersionType = http.HttpConnectionManager_CodecType

const (
	HTTPVersionAuto HTTPVersionType = http.HttpConnectionManager_AUTO
	HTTPVersion1    HTTPVersionType = http.HttpConnectionManager_HTTP1
	HTTPVersion2    HTTPVersionType = http.HttpConnectionManager_HTTP2
	HTTPVersion3    HTTPVersionType = http.HttpConnectionManager_HTTP3

	HTTPFilterRouter  = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
	HTTPFilterCORS    = "type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors"
	HTTPFilterGrpcWeb = "type.googleapis.com/envoy.extensions.filters.http.grpc_web.v3.GrpcWeb"
	HTTPFilterGzip    = "type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip"
)

// ProtoNamesForVersions returns the slice of ALPN protocol names for the give HTTP versions.
func ProtoNamesForVersions(versions ...HTTPVersionType) []string {
	protocols := map[HTTPVersionType]string{
		HTTPVersion1: "http/1.1",
		HTTPVersion2: "h2",
		HTTPVersion3: "",
	}
	defaultVersions := []string{"h2", "http/1.1"}
	wantedVersions := map[HTTPVersionType]struct{}{}

	if versions == nil {
		return defaultVersions
	}

	for _, v := range versions {
		wantedVersions[v] = struct{}{}
	}

	var alpn []string

	// Check for versions in preference order.
	for _, v := range []HTTPVersionType{HTTPVersionAuto, HTTPVersion2, HTTPVersion1} {
		if _, ok := wantedVersions[v]; ok {
			if v == HTTPVersionAuto {
				return defaultVersions
			}

			log.Printf("wanted %d -> %s", v, protocols[v])
			alpn = append(alpn, protocols[v])
		}
	}

	return alpn
}

// CodecForVersions determines a single Envoy HTTP codec constant
// that support all the given HTTP protocol versions.
func CodecForVersions(versions ...HTTPVersionType) HTTPVersionType {
	switch len(versions) {
	case 1:
		return versions[0]
	case 0:
		// Default is to autodetect.
		return HTTPVersionAuto
	default:
		// If more than one version is allowed, autodetect and let ALPN sort it out.
		return HTTPVersionAuto
	}
}

// TLSInspector returns a new TLS inspector listener filter.
func TLSInspector() *envoy_listener_v3.ListenerFilter {
	return &envoy_listener_v3.ListenerFilter{
		Name: wellknown.TlsInspector,
	}
}

// ProxyProtocol returns a new Proxy Protocol listener filter.
func ProxyProtocol() *envoy_listener_v3.ListenerFilter {
	return &envoy_listener_v3.ListenerFilter{
		Name: wellknown.ProxyProtocol,
	}
}

// Listener returns a new envoy_listener_v3.Listener for the supplied address, port, and filters.
func Listener(name, address string, port int, lf []*envoy_listener_v3.ListenerFilter, filters ...*envoy_listener_v3.Filter) *envoy_listener_v3.Listener {
	l := &envoy_listener_v3.Listener{
		Name:            name,
		Address:         SocketAddress(address, port),
		ListenerFilters: lf,
		SocketOptions:   TCPKeepaliveSocketOptions(),
	}
	if len(filters) > 0 {
		l.FilterChains = append(
			l.FilterChains,
			&envoy_listener_v3.FilterChain{
				Filters: filters,
			},
		)
	}
	return l
}

type httpConnectionManagerBuilder struct {
	routeConfigName               string
	metricsPrefix                 string
	accessLoggers                 []*accesslog.AccessLog
	requestTimeout                timeout.Setting
	connectionIdleTimeout         timeout.Setting
	streamIdleTimeout             timeout.Setting
	delayedCloseTimeout           timeout.Setting
	maxConnectionDuration         timeout.Setting
	connectionShutdownGracePeriod timeout.Setting
	filters                       []*http.HttpFilter
	codec                         HTTPVersionType // Note the zero value is AUTO, which is the default we want.
	allowChunkedLength            bool
	numTrustedHops                uint32
}

// RouteConfigName sets the name of the RDS element that contains
// the routing table for this manager.
func (b *httpConnectionManagerBuilder) RouteConfigName(name string) *httpConnectionManagerBuilder {
	b.routeConfigName = name
	return b
}

// MetricsPrefix sets the prefix used for emitting metrics from the
// connection manager. Note that this prefix is externally visible in
// monitoring tools, so it is subject to compatibility concerns.
//
// See https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/stats#config-http-conn-man-stats
func (b *httpConnectionManagerBuilder) MetricsPrefix(prefix string) *httpConnectionManagerBuilder {
	b.metricsPrefix = prefix
	return b
}

// Codec sets the HTTP codec for the manager. The default is AUTO.
func (b *httpConnectionManagerBuilder) Codec(codecType HTTPVersionType) *httpConnectionManagerBuilder {
	b.codec = codecType
	return b
}

// AccessLoggers sets the access logging configuration.
func (b *httpConnectionManagerBuilder) AccessLoggers(loggers []*accesslog.AccessLog) *httpConnectionManagerBuilder {
	b.accessLoggers = loggers
	return b
}

// RequestTimeout sets the active request timeout on the connection manager.
func (b *httpConnectionManagerBuilder) RequestTimeout(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.requestTimeout = timeout
	return b
}

// ConnectionIdleTimeout sets the idle timeout on the connection manager.
func (b *httpConnectionManagerBuilder) ConnectionIdleTimeout(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.connectionIdleTimeout = timeout
	return b
}

// StreamIdleTimeout sets the stream idle timeout on the connection manager.
func (b *httpConnectionManagerBuilder) StreamIdleTimeout(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.streamIdleTimeout = timeout
	return b
}

// DelayedCloseTimeout sets the delayed close timeout on the connection manager.
func (b *httpConnectionManagerBuilder) DelayedCloseTimeout(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.delayedCloseTimeout = timeout
	return b
}

// MaxConnectionDuration sets the max connection duration on the connection manager.
func (b *httpConnectionManagerBuilder) MaxConnectionDuration(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.maxConnectionDuration = timeout
	return b
}

// ConnectionShutdownGracePeriod sets the drain timeout on the connection manager.
func (b *httpConnectionManagerBuilder) ConnectionShutdownGracePeriod(timeout timeout.Setting) *httpConnectionManagerBuilder {
	b.connectionShutdownGracePeriod = timeout
	return b
}

func (b *httpConnectionManagerBuilder) AllowChunkedLength(enabled bool) *httpConnectionManagerBuilder {
	b.allowChunkedLength = enabled
	return b
}

func (b *httpConnectionManagerBuilder) NumTrustedHops(num uint32) *httpConnectionManagerBuilder {
	b.numTrustedHops = num
	return b
}

func (b *httpConnectionManagerBuilder) DefaultFilters() *httpConnectionManagerBuilder {

	// Add a default set of ordered http filters.
	// The names are not required to match anything and are
	// identified by the TypeURL of each filter.
	b.filters = append(b.filters,
		&http.HttpFilter{
			Name: "compressor",
			ConfigType: &http.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
					CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
						Name: "gzip",
						TypedConfig: &any.Any{
							TypeUrl: HTTPFilterGzip,
						},
					},
				}),
			},
		},
		&http.HttpFilter{
			Name: "grpcweb",
			ConfigType: &http.HttpFilter_TypedConfig{
				TypedConfig: &any.Any{
					TypeUrl: HTTPFilterGrpcWeb,
				},
			},
		},
		&http.HttpFilter{
			Name: "cors",
			ConfigType: &http.HttpFilter_TypedConfig{
				TypedConfig: &any.Any{
					TypeUrl: HTTPFilterCORS,
				},
			},
		},
		&http.HttpFilter{
			Name: "local_ratelimit",
			ConfigType: &http.HttpFilter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(
					&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
						StatPrefix: "http",
						// since no token bucket is defined here, the filter is disabled
						// globally but can be enabled on a per-vhost/route basis.
					},
				),
			},
		},
		&http.HttpFilter{
			Name: "router",
			ConfigType: &http.HttpFilter_TypedConfig{
				TypedConfig: &any.Any{
					TypeUrl: HTTPFilterRouter,
				},
			},
		},
	)

	return b
}

// AddFilter appends f to the list of filters for this HTTPConnectionManager. f
// may be nil, in which case it is ignored. Note that Router filters
// (filters with TypeUrl `type.googleapis.com/envoy.extensions.filters.http.router.v3.Router`)
// are specially treated. There may only be one of these filters, and it must be the last.
// AddFilter will ensure that the router filter, if present, is last, and will panic
// if a second Router is added when one is already present.
func (b *httpConnectionManagerBuilder) AddFilter(f *http.HttpFilter) *httpConnectionManagerBuilder {
	if f == nil {
		return b
	}

	b.filters = append(b.filters, f)

	if len(b.filters) == 1 {
		return b
	}

	lastIndex := len(b.filters) - 1
	routerIndex := -1
	for i, filter := range b.filters {
		if filter.GetTypedConfig().MessageIs(&envoy_extensions_filters_http_router_v3.Router{}) {
			routerIndex = i
			break
		}
	}

	// We can't add more than one router entry, and there should be no way to do it.
	// If this happens, it has to be programmer error, so we panic to tell them
	// it needs to be fixed. Note that in hitting this case, it doesn't matter we added
	// the second one earlier, because we're panicking anyway.
	if routerIndex != -1 && f.GetTypedConfig().MessageIs(&envoy_extensions_filters_http_router_v3.Router{}) {
		panic("Can't add more than one router to a filter chain")
	}

	if routerIndex != lastIndex {
		// Move the router to the end of the filters array.
		routerFilter := b.filters[routerIndex]
		b.filters = append(b.filters[:routerIndex], b.filters[routerIndex+1])
		b.filters = append(b.filters, routerFilter)
	}

	return b
}

// Validate runs builtin validation rules against the current builder state.
func (b *httpConnectionManagerBuilder) Validate() error {

	// It's not OK for the filters to be empty.
	if len(b.filters) == 0 {
		return errors.New("filter list is empty")
	}

	// If the router filter is not the last, the listener will be rejected by Envoy.
	// More specifically, the last filter must be a terminating filter. The only one
	// of these used by Contour is the router filter, which is set as the one
	// with typeUrl `type.googleapis.com/envoy.extensions.filters.http.router.v3.Router`,
	// which in this case is the one of type Router.
	lastIndex := len(b.filters) - 1
	if !b.filters[lastIndex].GetTypedConfig().MessageIs(&envoy_extensions_filters_http_router_v3.Router{}) {
		return errors.New("last filter is not a Router filter")
	}

	return nil
}

// Get returns a new http.HttpConnectionManager filter, constructed
// from the builder settings.
//
// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto
func (b *httpConnectionManagerBuilder) Get() *envoy_listener_v3.Filter {
	// For now, failing validation is a programmer error that
	// the caller can't reasonably recover from. A caller that can
	// handle this should validate manually.
	if err := b.Validate(); err != nil {
		panic(err.Error())
	}

	cm := &http.HttpConnectionManager{
		CodecType: b.codec,
		RouteSpecifier: &http.HttpConnectionManager_Rds{
			Rds: &http.Rds{
				RouteConfigName: b.routeConfigName,
				ConfigSource:    ConfigSource("contour"),
			},
		},
		HttpFilters: b.filters,
		CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{
			IdleTimeout: envoy.Timeout(b.connectionIdleTimeout),
		},
		HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
			// Enable support for HTTP/1.0 requests that carry
			// a Host: header. See #537.
			AcceptHttp_10:      true,
			AllowChunkedLength: b.allowChunkedLength,
		},
		UseRemoteAddress: protobuf.Bool(true),
		NormalizePath:    protobuf.Bool(true),

		// We can ignore any port number supplied in the Host/:authority header
		// before processing by filters or routing.
		// Note that the port a listener is bound to will already be selected
		// and that the port is stripped from the header sent upstream as well.
		StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
			StripAnyHostPort: true,
		},

		// issue #1487 pass through X-Request-Id if provided.
		PreserveExternalRequestId: true,
		MergeSlashes:              true,

		RequestTimeout:      envoy.Timeout(b.requestTimeout),
		StreamIdleTimeout:   envoy.Timeout(b.streamIdleTimeout),
		DrainTimeout:        envoy.Timeout(b.connectionShutdownGracePeriod),
		DelayedCloseTimeout: envoy.Timeout(b.delayedCloseTimeout),
		XffNumTrustedHops:   b.numTrustedHops,
	}

	// Max connection duration is infinite/disabled by default in Envoy, so if the timeout setting
	// indicates to either disable or use default, don't pass a value at all. Note that unlike other
	// Envoy timeouts, explicitly passing a 0 here *would not* disable the timeout; it needs to be
	// omitted entirely.
	if !b.maxConnectionDuration.IsDisabled() && !b.maxConnectionDuration.UseDefault() {
		cm.CommonHttpProtocolOptions.MaxConnectionDuration = protobuf.Duration(b.maxConnectionDuration.Duration())
	}

	if len(b.accessLoggers) > 0 {
		cm.AccessLog = b.accessLoggers
	}

	// If there's no explicit metrics prefix, default it to the
	// route config name.
	if b.metricsPrefix != "" {
		cm.StatPrefix = b.metricsPrefix
	} else {
		cm.StatPrefix = b.routeConfigName
	}

	return &envoy_listener_v3.Filter{
		Name: wellknown.HTTPConnectionManager,
		ConfigType: &envoy_listener_v3.Filter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(cm),
		},
	}
}

// HTTPConnectionManager creates a new HTTP Connection Manager filter
// for the supplied route, access log, and client request timeout.
func HTTPConnectionManager(routename string, accesslogger []*accesslog.AccessLog, requestTimeout time.Duration, xffNumTrustedHops uint32) *envoy_listener_v3.Filter {
	return HTTPConnectionManagerBuilder().
		RouteConfigName(routename).
		MetricsPrefix(routename).
		AccessLoggers(accesslogger).
		RequestTimeout(timeout.DurationSetting(requestTimeout)).
		NumTrustedHops(xffNumTrustedHops).
		DefaultFilters().
		Get()
}

// HTTPConnectionManagerBuilder creates a new HTTP connection manager builder.
// nolint:revive
func HTTPConnectionManagerBuilder() *httpConnectionManagerBuilder {
	return &httpConnectionManagerBuilder{}
}

// TCPProxy creates a new TCPProxy filter.
func TCPProxy(statPrefix string, proxy *dag.TCPProxy, accesslogger []*accesslog.AccessLog) *envoy_listener_v3.Filter {
	// Set the idle timeout in seconds for connections through a TCP Proxy type filter.
	// The value of two and a half hours for reasons documented at
	// https://github.com/projectcontour/contour/issues/1074
	// Set to 9001 because now it's OVER NINE THOUSAND.
	idleTimeout := protobuf.Duration(9001 * time.Second)

	switch len(proxy.Clusters) {
	case 1:
		return &envoy_listener_v3.Filter{
			Name: wellknown.TCPProxy,
			ConfigType: &envoy_listener_v3.Filter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&tcp.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &tcp.TcpProxy_Cluster{
						Cluster: envoy.Clustername(proxy.Clusters[0]),
					},
					AccessLog:   accesslogger,
					IdleTimeout: idleTimeout,
				}),
			},
		}
	default:
		var clusters []*tcp.TcpProxy_WeightedCluster_ClusterWeight
		for _, c := range proxy.Clusters {
			weight := c.Weight
			if weight == 0 {
				weight = 1
			}
			clusters = append(clusters, &tcp.TcpProxy_WeightedCluster_ClusterWeight{
				Name:   envoy.Clustername(c),
				Weight: weight,
			})
		}
		sort.Stable(sorter.For(clusters))
		return &envoy_listener_v3.Filter{
			Name: wellknown.TCPProxy,
			ConfigType: &envoy_listener_v3.Filter_TypedConfig{
				TypedConfig: protobuf.MustMarshalAny(&tcp.TcpProxy{
					StatPrefix: statPrefix,
					ClusterSpecifier: &tcp.TcpProxy_WeightedClusters{
						WeightedClusters: &tcp.TcpProxy_WeightedCluster{
							Clusters: clusters,
						},
					},
					AccessLog:   accesslogger,
					IdleTimeout: idleTimeout,
				}),
			},
		}
	}
}

// SocketAddress creates a new TCP envoy_core_v3.Address.
func SocketAddress(address string, port int) *envoy_core_v3.Address {
	if address == "::" {
		return &envoy_core_v3.Address{
			Address: &envoy_core_v3.Address_SocketAddress{
				SocketAddress: &envoy_core_v3.SocketAddress{
					Protocol:   envoy_core_v3.SocketAddress_TCP,
					Address:    address,
					Ipv4Compat: true,
					PortSpecifier: &envoy_core_v3.SocketAddress_PortValue{
						PortValue: uint32(port),
					},
				},
			},
		}
	}
	return &envoy_core_v3.Address{
		Address: &envoy_core_v3.Address_SocketAddress{
			SocketAddress: &envoy_core_v3.SocketAddress{
				Protocol: envoy_core_v3.SocketAddress_TCP,
				Address:  address,
				PortSpecifier: &envoy_core_v3.SocketAddress_PortValue{
					PortValue: uint32(port),
				},
			},
		},
	}
}

// Filters returns a []*envoy_listener_v3.Filter for the supplied filters.
func Filters(filters ...*envoy_listener_v3.Filter) []*envoy_listener_v3.Filter {
	if len(filters) == 0 {
		return nil
	}
	return filters
}

// FilterChain returns a *envoy_listener_v3.FilterChain for the supplied filters.
func FilterChain(filters ...*envoy_listener_v3.Filter) *envoy_listener_v3.FilterChain {
	return &envoy_listener_v3.FilterChain{
		Filters: filters,
	}
}

// FilterChains returns a []*envoy_listener_v3.FilterChain for the supplied filters.
func FilterChains(filters ...*envoy_listener_v3.Filter) []*envoy_listener_v3.FilterChain {
	if len(filters) == 0 {
		return nil
	}
	return []*envoy_listener_v3.FilterChain{
		FilterChain(filters...),
	}
}

func FilterMisdirectedRequests(fqdn string) *http.HttpFilter {
	var target string

	if strings.HasPrefix(fqdn, "*.") {
		// When we have a wildcard hostname, we will have already matched
		// the filter chain on an SNI that falls under the wildcard so we
		// retrieve that and make sure the :authority header matches.
		// See: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/lua_filter#requestedservername
		target = "request_handle:streamInfo():requestedServerName()"
	} else {
		// For specific hostnames we know the SNI we need to match the
		// :authority header against so we can simplify the code.
		target = `"` + strings.ToLower(fqdn) + `"`
	}

	code := `
function envoy_on_request(request_handle)
	local headers = request_handle:headers()
	local host = string.lower(headers:get(":authority"))
	local target = %s

	if host ~= target then
		request_handle:respond(
			{[":status"] = "421"},
			string.format("misdirected request to %%q", host)
		)
	end
end
	`

	return &http.HttpFilter{
		Name: "envoy.filters.http.lua",
		ConfigType: &http.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&lua.Lua{
				InlineCode: fmt.Sprintf(code, target),
			}),
		},
	}
}

// FilterExternalAuthz returns an `ext_authz` filter configured with the
// requested parameters.
func FilterExternalAuthz(authzClusterName string, failOpen bool, timeout timeout.Setting) *http.HttpFilter {
	authConfig := envoy_config_filter_http_ext_authz_v3.ExtAuthz{
		Services: &envoy_config_filter_http_ext_authz_v3.ExtAuthz_GrpcService{
			GrpcService: &envoy_core_v3.GrpcService{
				TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
						ClusterName: authzClusterName,
					},
				},
				Timeout: envoy.Timeout(timeout),
				// We don't need to configure metadata here, since we allow
				// operators to specify authorization context parameters at
				// the virtual host and route.
				InitialMetadata: []*envoy_core_v3.HeaderValue{},
			},
		},
		// Pretty sure we always want this. Why have an
		// external auth service if it is not going to affect
		// routing decisions?
		ClearRouteCache:  true,
		FailureModeAllow: failOpen,
		StatusOnError: &envoy_type.HttpStatus{
			Code: envoy_type.StatusCode_Forbidden,
		},
		MetadataContextNamespaces: []string{},
		IncludePeerCertificate:    true,
		// TODO(jpeach): When we move to the Envoy v4 API, propagate the
		// `transport_api_version` from ExtensionServiceSpec ProtocolVersion.
		TransportApiVersion: envoy_core_v3.ApiVersion_V3,
	}

	return &http.HttpFilter{
		Name: "envoy.filters.http.ext_authz",
		ConfigType: &http.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&authConfig),
		},
	}
}

// FilterChainTLS returns a TLS enabled envoy_listener_v3.FilterChain.
func FilterChainTLS(domain string, downstream *envoy_tls_v3.DownstreamTlsContext, filters []*envoy_listener_v3.Filter) *envoy_listener_v3.FilterChain {
	fc := &envoy_listener_v3.FilterChain{
		Filters: filters,
	}

	// If the domain doesn't have a specific SNI, Envoy can't filter
	// on that, so change the Match to be on TransportProtocol which would
	// match any request over TLS to this listener.
	if domain == "*" {
		fc.FilterChainMatch = &envoy_listener_v3.FilterChainMatch{
			TransportProtocol: "tls",
		}
	} else {
		fc.FilterChainMatch = &envoy_listener_v3.FilterChainMatch{
			ServerNames: []string{domain},
		}
	}

	// Attach TLS data to this listener if provided.
	if downstream != nil {
		fc.TransportSocket = DownstreamTLSTransportSocket(downstream)

	}
	return fc
}

// FilterChainTLSFallback returns a TLS enabled envoy_listener_v3.FilterChain conifgured for FallbackCertificate.
func FilterChainTLSFallback(downstream *envoy_tls_v3.DownstreamTlsContext, filters []*envoy_listener_v3.Filter) *envoy_listener_v3.FilterChain {
	fc := &envoy_listener_v3.FilterChain{
		Name:    "fallback-certificate",
		Filters: filters,
		FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
			TransportProtocol: "tls",
		},
	}
	// Attach TLS data to this listener if provided.
	if downstream != nil {
		fc.TransportSocket = DownstreamTLSTransportSocket(downstream)
	}
	return fc
}

// ListenerFilters returns a []*envoy_listener_v3.ListenerFilter for the supplied listener filters.
func ListenerFilters(filters ...*envoy_listener_v3.ListenerFilter) []*envoy_listener_v3.ListenerFilter {
	return filters
}

func ContainsFallbackFilterChain(filterchains []*envoy_listener_v3.FilterChain) bool {
	for _, fc := range filterchains {
		if fc.Name == "fallback-certificate" {
			return true
		}
	}
	return false
}
