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
	"sort"
	"sync"

	envoy_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/pkg/config"
	"k8s.io/apimachinery/pkg/types"
)

// nolint:golint
const (
	ENVOY_HTTP_LISTENER            = "ingress_http"
	ENVOY_FALLBACK_ROUTECONFIG     = "ingress_fallbackcert"
	ENVOY_HTTPS_LISTENER           = "ingress_https"
	DEFAULT_HTTP_ACCESS_LOG        = "/dev/stdout"
	DEFAULT_HTTP_LISTENER_ADDRESS  = "0.0.0.0"
	DEFAULT_HTTP_LISTENER_PORT     = 8080
	DEFAULT_HTTPS_ACCESS_LOG       = "/dev/stdout"
	DEFAULT_HTTPS_LISTENER_ADDRESS = DEFAULT_HTTP_LISTENER_ADDRESS
	DEFAULT_HTTPS_LISTENER_PORT    = 8443
)

type Listener struct {
	Name    string
	Address string
	Port    int
}

// ListenerConfig holds configuration parameters for building Envoy Listeners.
type ListenerConfig struct {

	// Envoy's HTTP (non TLS) listener addresses.
	// If not set, defaults to a single listener with
	// DEFAULT_HTTP_LISTENER_ADDRESS:DEFAULT_HTTP_LISTENER_PORT.
	HTTPListeners map[string]Listener

	// Envoy's HTTP (non TLS) access log path.
	// If not set, defaults to DEFAULT_HTTP_ACCESS_LOG.
	HTTPAccessLog string

	// Envoy's HTTPS (TLS) listener addresses.
	// If not set, defaults to a single listener with
	// DEFAULT_HTTPS_LISTENER_ADDRESS:DEFAULT_HTTPS_LISTENER_PORT.
	HTTPSListeners map[string]Listener

	// Envoy's HTTPS (TLS) access log path.
	// If not set, defaults to DEFAULT_HTTPS_ACCESS_LOG.
	HTTPSAccessLog string

	// UseProxyProto configures all listeners to expect a PROXY
	// V1 or V2 preamble.
	// If not set, defaults to false.
	UseProxyProto bool

	// MinimumTLSVersion defines the minimum TLS protocol version the proxy should accept.
	MinimumTLSVersion string

	// CipherSuites defines the ciphers Envoy TLS listeners will accept when
	// negotiating TLS 1.2.
	CipherSuites []string

	// DefaultHTTPVersions defines the default set of HTTP
	// versions the proxy should accept. If not specified, all
	// supported versions are accepted. This is applied to both
	// HTTP and HTTPS listeners but has practical effect only for
	// HTTPS, because we don't support h2c.
	DefaultHTTPVersions []envoy_v3.HTTPVersionType

	// AccessLogType defines if Envoy logs should be output as Envoy's default or JSON.
	// Valid values: 'envoy', 'json'
	// If not set, defaults to 'envoy'
	AccessLogType config.AccessLogType

	// AccessLogFields sets the fields that should be shown in JSON logs.
	// Valid entries are the keys from internal/envoy/accesslog.go:jsonheaders
	// Defaults to a particular set of fields.
	AccessLogFields config.AccessLogFields

	// RequestTimeout configures the request_timeout for all Connection Managers.
	RequestTimeout timeout.Setting

	// ConnectionIdleTimeout configures the common_http_protocol_options.idle_timeout for all
	// Connection Managers.
	ConnectionIdleTimeout timeout.Setting

	// StreamIdleTimeout configures the stream_idle_timeout for all Connection Managers.
	StreamIdleTimeout timeout.Setting

	// DelayedCloseTimeout configures the delayed_close_timeout for all Connection Managers.
	DelayedCloseTimeout timeout.Setting

	// MaxConnectionDuration configures the common_http_protocol_options.max_connection_duration for all
	// Connection Managers.
	MaxConnectionDuration timeout.Setting

	// ConnectionShutdownGracePeriod configures the drain_timeout for all Connection Managers.
	ConnectionShutdownGracePeriod timeout.Setting

	// AllowChunkedLength enables setting allow_chunked_length on the HTTP1 options for all
	// listeners.
	AllowChunkedLength bool

	// XffNumTrustedHops sets the number of additional ingress proxy hops from the
	// right side of the x-forwarded-for HTTP header to trust.
	XffNumTrustedHops uint32

	// ConnectionBalancer
	// The validated value is 'exact'.
	// If no configuration is specified, Envoy will not attempt to balance active connections between worker threads
	// If specified, the listener will use the exact connection balancer.
	ConnectionBalancer string
	// RateLimitConfig optionally configures the global Rate Limit Service to be
	// used.
	RateLimitConfig *RateLimitConfig
}

type RateLimitConfig struct {
	ExtensionService        types.NamespacedName
	Domain                  string
	Timeout                 timeout.Setting
	FailOpen                bool
	EnableXRateLimitHeaders bool
}

// DefaultListeners returns the configured Listeners or a single
// Insecure (http) & single Secure (https) default listeners
// if not provided.
func (lvc *ListenerConfig) DefaultListeners() *ListenerConfig {

	httpListeners := lvc.HTTPListeners
	httpsListeners := lvc.HTTPSListeners

	if len(lvc.HTTPListeners) == 0 {
		httpListeners = map[string]Listener{
			ENVOY_HTTP_LISTENER: {
				Name:    ENVOY_HTTP_LISTENER,
				Address: DEFAULT_HTTP_LISTENER_ADDRESS,
				Port:    DEFAULT_HTTP_LISTENER_PORT,
			},
		}
	}

	if len(lvc.HTTPSListeners) == 0 {
		httpsListeners = map[string]Listener{
			ENVOY_HTTPS_LISTENER: {
				Name:    ENVOY_HTTPS_LISTENER,
				Address: DEFAULT_HTTPS_LISTENER_ADDRESS,
				Port:    DEFAULT_HTTPS_LISTENER_PORT,
			},
		}
	}

	lvc.HTTPListeners = httpListeners
	lvc.HTTPSListeners = httpsListeners
	return lvc
}

func (lvc *ListenerConfig) SecureListeners() map[string]*envoy_listener_v3.Listener {
	listeners := make(map[string]*envoy_listener_v3.Listener)

	if len(lvc.HTTPSListeners) == 0 {
		listeners[ENVOY_HTTPS_LISTENER] = envoy_v3.Listener(
			ENVOY_HTTPS_LISTENER,
			DEFAULT_HTTPS_LISTENER_ADDRESS,
			DEFAULT_HTTPS_LISTENER_PORT,
			secureProxyProtocol(lvc.UseProxyProto),
		)
	}

	for name, l := range lvc.HTTPSListeners {
		listeners[name] = envoy_v3.Listener(
			l.Name,
			l.Address,
			l.Port,
			secureProxyProtocol(lvc.UseProxyProto),
		)
	}

	return listeners
}

// httpAccessLog returns the access log for the HTTP (non TLS)
// listener or DEFAULT_HTTP_ACCESS_LOG if not configured.
func (lvc *ListenerConfig) httpAccessLog() string {
	if lvc.HTTPAccessLog != "" {
		return lvc.HTTPAccessLog
	}
	return DEFAULT_HTTP_ACCESS_LOG
}

// httpsAccessLog returns the access log for the HTTPS (TLS)
// listener or DEFAULT_HTTPS_ACCESS_LOG if not configured.
func (lvc *ListenerConfig) httpsAccessLog() string {
	if lvc.HTTPSAccessLog != "" {
		return lvc.HTTPSAccessLog
	}
	return DEFAULT_HTTPS_ACCESS_LOG
}

// accesslogType returns the access log type that should be configured
// across all listener types or DEFAULT_ACCESS_LOG_TYPE if not configured.
func (lvc *ListenerConfig) accesslogType() string {
	if lvc.AccessLogType != "" {
		return string(lvc.AccessLogType)
	}
	return string(config.DEFAULT_ACCESS_LOG_TYPE)
}

// accesslogFields returns the access log fields that should be configured
// for Envoy, or a default set if not configured.
func (lvc *ListenerConfig) accesslogFields() config.AccessLogFields {
	if lvc.AccessLogFields != nil {
		return lvc.AccessLogFields
	}
	return config.DefaultFields
}

func (lvc *ListenerConfig) newInsecureAccessLog() []*envoy_accesslog_v3.AccessLog {
	switch lvc.accesslogType() {
	case string(config.JSONAccessLog):
		return envoy_v3.FileAccessLogJSON(lvc.httpAccessLog(), lvc.accesslogFields())
	default:
		return envoy_v3.FileAccessLogEnvoy(lvc.httpAccessLog())
	}
}

func (lvc *ListenerConfig) newSecureAccessLog() []*envoy_accesslog_v3.AccessLog {
	switch lvc.accesslogType() {
	case "json":
		return envoy_v3.FileAccessLogJSON(lvc.httpsAccessLog(), lvc.accesslogFields())
	default:
		return envoy_v3.FileAccessLogEnvoy(lvc.httpsAccessLog())
	}
}

// minTLSVersion returns the requested minimum TLS protocol
// version or envoy_tls_v3.TlsParameters_TLSv1_2 if not configured.
func (lvc *ListenerConfig) minTLSVersion() envoy_tls_v3.TlsParameters_TlsProtocol {
	minTLSVersion := envoy_v3.ParseTLSVersion(lvc.MinimumTLSVersion)
	if minTLSVersion > envoy_tls_v3.TlsParameters_TLSv1_2 {
		return minTLSVersion
	}
	return envoy_tls_v3.TlsParameters_TLSv1_2
}

// ListenerCache manages the contents of the gRPC LDS cache.
type ListenerCache struct {
	mu           sync.Mutex
	values       map[string]*envoy_listener_v3.Listener
	staticValues map[string]*envoy_listener_v3.Listener

	Config ListenerConfig
	contour.Cond
}

// NewListenerCache returns an instance of a ListenerCache
func NewListenerCache(config ListenerConfig, address string, port int) *ListenerCache {
	stats := envoy_v3.StatsListener(address, port)
	return &ListenerCache{
		Config: config,
		staticValues: map[string]*envoy_listener_v3.Listener{
			stats.Name: stats,
		},
	}
}

// Update replaces the contents of the cache with the supplied map.
func (c *ListenerCache) Update(v map[string]*envoy_listener_v3.Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.Cond.Notify()
}

// Contents returns a copy of the cache's contents.
func (c *ListenerCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_listener_v3.Listener
	for _, v := range c.values {
		values = append(values, v)
	}
	for _, v := range c.staticValues {
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

// Query returns the proto.Messages in the ListenerCache that match
// a slice of strings
func (c *ListenerCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_listener_v3.Listener
	for _, n := range names {
		v, ok := c.values[n]
		if !ok {
			v, ok = c.staticValues[n]
			if !ok {
				// if the listener is not registered in
				// dynamic or static values then skip it
				// as there is no way to return a blank
				// listener because the listener address
				// field is required.
				continue
			}
		}
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*ListenerCache) TypeURL() string { return resource.ListenerType }

func (c *ListenerCache) OnChange(root *dag.DAG) {
	listeners := visitListeners(root, &c.Config)
	c.Update(listeners)
}

type listenerVisitor struct {
	*ListenerConfig

	listeners        map[string]*envoy_listener_v3.Listener
	httpListenerName string // Name of dag.VirtualHost encountered.
}

func visitListeners(root dag.Vertex, lvc *ListenerConfig) map[string]*envoy_listener_v3.Listener {
	lv := listenerVisitor{
		ListenerConfig: lvc.DefaultListeners(),
		listeners:      lvc.SecureListeners(),
	}

	lv.visit(root)

	if httpListener, ok := lvc.HTTPListeners[lv.httpListenerName]; ok {

		// Add a listener if there are vhosts bound to http.
		cm := envoy_v3.HTTPConnectionManagerBuilder().
			Codec(envoy_v3.CodecForVersions(lv.DefaultHTTPVersions...)).
			DefaultFilters().
			RouteConfigName(httpListener.Name).
			MetricsPrefix(httpListener.Name).
			AccessLoggers(lvc.newInsecureAccessLog()).
			RequestTimeout(lvc.RequestTimeout).
			ConnectionIdleTimeout(lvc.ConnectionIdleTimeout).
			StreamIdleTimeout(lvc.StreamIdleTimeout).
			DelayedCloseTimeout(lvc.DelayedCloseTimeout).
			MaxConnectionDuration(lvc.MaxConnectionDuration).
			ConnectionShutdownGracePeriod(lvc.ConnectionShutdownGracePeriod).
			AllowChunkedLength(lvc.AllowChunkedLength).
			NumTrustedHops(lvc.XffNumTrustedHops).
			AddFilter(envoy_v3.GlobalRateLimitFilter(envoyGlobalRateLimitConfig(lv.RateLimitConfig))).
			Get()

		lv.listeners[httpListener.Name] = envoy_v3.Listener(
			httpListener.Name,
			httpListener.Address,
			httpListener.Port,
			proxyProtocol(lvc.UseProxyProto),
			cm,
		)
	}

	// Remove the https listener if there are no vhosts bound to it.
	if len(lv.listeners[ENVOY_HTTPS_LISTENER].FilterChains) == 0 {
		delete(lv.listeners, ENVOY_HTTPS_LISTENER)
	} else {
		// there's some https listeners, we need to sort the filter chains
		// to ensure that the LDS entries are identical.
		sort.Stable(sorter.For(lv.listeners[ENVOY_HTTPS_LISTENER].FilterChains))
	}

	// support more params of envoy listener

	// 1. connection balancer
	switch lvc.ConnectionBalancer {
	case "exact":
		for _, listener := range lv.listeners {
			listener.ConnectionBalanceConfig = &envoy_listener_v3.Listener_ConnectionBalanceConfig{
				BalanceType: &envoy_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance_{
					ExactBalance: &envoy_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance{},
				},
			}
		}
	}

	return lv.listeners
}

func envoyGlobalRateLimitConfig(config *RateLimitConfig) *envoy_v3.GlobalRateLimitConfig {
	if config == nil {
		return nil
	}

	return &envoy_v3.GlobalRateLimitConfig{
		ExtensionService:        config.ExtensionService,
		FailOpen:                config.FailOpen,
		Timeout:                 config.Timeout,
		Domain:                  config.Domain,
		EnableXRateLimitHeaders: config.EnableXRateLimitHeaders,
	}
}

func proxyProtocol(useProxy bool) []*envoy_listener_v3.ListenerFilter {
	if useProxy {
		return envoy_v3.ListenerFilters(
			envoy_v3.ProxyProtocol(),
		)
	}
	return nil
}

func secureProxyProtocol(useProxy bool) []*envoy_listener_v3.ListenerFilter {
	return append(proxyProtocol(useProxy), envoy_v3.TLSInspector())
}

func (v *listenerVisitor) visit(vertex dag.Vertex) {
	max := func(a, b envoy_tls_v3.TlsParameters_TlsProtocol) envoy_tls_v3.TlsParameters_TlsProtocol {
		if a > b {
			return a
		}
		return b
	}

	switch vh := vertex.(type) {
	case *dag.VirtualHost:
		// we only create on http listener so record the fact
		// that we need to then double back at the end and add
		// the listener properly
		v.httpListenerName = vh.ListenerName
	case *dag.SecureVirtualHost:
		var alpnProtos []string
		var filters []*envoy_listener_v3.Filter

		if vh.TCPProxy == nil {
			var authFilter *http.HttpFilter

			if vh.AuthorizationService != nil {
				authFilter = envoy_v3.FilterExternalAuthz(
					vh.AuthorizationService.Name,
					vh.AuthorizationFailOpen,
					vh.AuthorizationResponseTimeout,
				)
			}

			// Create a uniquely named HTTP connection manager for
			// this vhost, so that the SNI name the client requests
			// only grants access to that host. See RFC 6066 for
			// security advice. Note that we still use the generic
			// metrics prefix to keep compatibility with previous
			// Contour versions since the metrics prefix will be
			// coded into monitoring dashboards.
			cm := envoy_v3.HTTPConnectionManagerBuilder().
				Codec(envoy_v3.CodecForVersions(v.DefaultHTTPVersions...)).
				AddFilter(envoy_v3.FilterMisdirectedRequests(vh.VirtualHost.Name)).
				DefaultFilters().
				AddFilter(authFilter).
				RouteConfigName(path.Join("https", vh.VirtualHost.Name)).
				MetricsPrefix(vh.ListenerName).
				AccessLoggers(v.ListenerConfig.newSecureAccessLog()).
				RequestTimeout(v.ListenerConfig.RequestTimeout).
				ConnectionIdleTimeout(v.ListenerConfig.ConnectionIdleTimeout).
				StreamIdleTimeout(v.ListenerConfig.StreamIdleTimeout).
				DelayedCloseTimeout(v.ListenerConfig.DelayedCloseTimeout).
				MaxConnectionDuration(v.ListenerConfig.MaxConnectionDuration).
				ConnectionShutdownGracePeriod(v.ListenerConfig.ConnectionShutdownGracePeriod).
				AllowChunkedLength(v.ListenerConfig.AllowChunkedLength).
				NumTrustedHops(v.ListenerConfig.XffNumTrustedHops).
				AddFilter(envoy_v3.GlobalRateLimitFilter(envoyGlobalRateLimitConfig(v.RateLimitConfig))).
				Get()

			filters = envoy_v3.Filters(cm)

			alpnProtos = envoy_v3.ProtoNamesForVersions(v.DefaultHTTPVersions...)
		} else {
			filters = envoy_v3.Filters(
				envoy_v3.TCPProxy(vh.ListenerName,
					vh.TCPProxy,
					v.ListenerConfig.newSecureAccessLog()),
			)

			// Do not offer ALPN for TCP proxying, since
			// the protocols will be provided by the TCP
			// backend in its ServerHello.
		}

		var downstreamTLS *envoy_tls_v3.DownstreamTlsContext

		// Secret is provided when TLS is terminated and nil when TLS passthrough is used.
		if len(vh.Secrets) > 0 {
			// Choose the higher of the configured or requested TLS version.
			vers := max(v.ListenerConfig.minTLSVersion(), envoy_v3.ParseTLSVersion(vh.MinTLSVersion))

			downstreamTLS = envoy_v3.DownstreamTLSContext(
				vh.Secrets,
				vers,
				v.ListenerConfig.CipherSuites,
				vh.DownstreamValidation,
				alpnProtos...)
		}

		v.listeners[vh.ListenerName].FilterChains = append(v.listeners[vh.ListenerName].FilterChains,
			envoy_v3.FilterChainTLS(vh.VirtualHost.Name, downstreamTLS, filters))

		// If this VirtualHost has enabled the fallback certificate then set a default
		// FilterChain which will allow routes with this vhost to accept non-SNI TLS requests.
		// Note that we don't add the misdirected requests filter on this chain because at this
		// point we don't actually know the full set of server names that will be bound to the
		// filter chain through the ENVOY_FALLBACK_ROUTECONFIG route configuration.
		if vh.FallbackCertificate != nil && !envoy_v3.ContainsFallbackFilterChain(v.listeners[vh.ListenerName].FilterChains) {
			// Construct the downstreamTLSContext passing the configured fallbackCertificate. The TLS minProtocolVersion will use
			// the value defined in the Contour Configuration file if defined.
			downstreamTLS = envoy_v3.DownstreamTLSContext(
				map[string]*dag.Secret{vh.FallbackCertificate.Name(): vh.FallbackCertificate},
				v.ListenerConfig.minTLSVersion(),
				v.ListenerConfig.CipherSuites,
				vh.DownstreamValidation,
				alpnProtos...)

			cm := envoy_v3.HTTPConnectionManagerBuilder().
				DefaultFilters().
				RouteConfigName(ENVOY_FALLBACK_ROUTECONFIG).
				MetricsPrefix(vh.ListenerName).
				AccessLoggers(v.ListenerConfig.newSecureAccessLog()).
				RequestTimeout(v.ListenerConfig.RequestTimeout).
				ConnectionIdleTimeout(v.ListenerConfig.ConnectionIdleTimeout).
				StreamIdleTimeout(v.ListenerConfig.StreamIdleTimeout).
				DelayedCloseTimeout(v.ListenerConfig.DelayedCloseTimeout).
				MaxConnectionDuration(v.ListenerConfig.MaxConnectionDuration).
				ConnectionShutdownGracePeriod(v.ListenerConfig.ConnectionShutdownGracePeriod).
				AllowChunkedLength(v.ListenerConfig.AllowChunkedLength).
				NumTrustedHops(v.ListenerConfig.XffNumTrustedHops).
				AddFilter(envoy_v3.GlobalRateLimitFilter(envoyGlobalRateLimitConfig(v.RateLimitConfig))).
				Get()

			// Default filter chain
			filters = envoy_v3.Filters(cm)

			v.listeners[vh.ListenerName].FilterChains = append(v.listeners[vh.ListenerName].FilterChains,
				envoy_v3.FilterChainTLSFallback(downstreamTLS, filters))
		}

	default:
		// recurse
		vertex.Visit(v.visit)
	}
}
