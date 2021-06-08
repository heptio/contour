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

// Package dag provides a data model, in the form of a directed acyclic graph,
// of the relationship between Kubernetes Ingress, Service, and Secret objects.
package dag

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/projectcontour/contour/internal/status"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/xds"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Vertex is a node in the DAG that can be visited.
type Vertex interface {
	Visit(func(Vertex))
}

// Observer is an interface for receiving notification of DAG updates.
type Observer interface {
	OnChange(*DAG)
}

// ObserverFunc is a function that implements the Observer interface
// by calling itself. It can be nil.
type ObserverFunc func(*DAG)

func (f ObserverFunc) OnChange(d *DAG) {
	if f != nil {
		f(d)
	}
}

var _ Observer = ObserverFunc(nil)

// ComposeObservers returns a new Observer that calls each of its arguments in turn.
func ComposeObservers(observers ...Observer) Observer {
	return ObserverFunc(func(d *DAG) {
		for _, o := range observers {
			o.OnChange(d)
		}
	})
}

// A DAG represents a directed acyclic graph of objects representing the relationship
// between Kubernetes Ingress objects, the backend Services, and Secret objects.
// The DAG models these relationships as Roots and Vertices.
type DAG struct {
	// StatusCache holds a cache of status updates to send.
	StatusCache status.Cache

	// roots are the root vertices of this DAG.
	roots []Vertex
}

// Visit calls fn on each root of this DAG.
func (d *DAG) Visit(fn func(Vertex)) {
	for _, r := range d.roots {
		fn(r)
	}
}

// AddRoot appends the given root to the DAG's roots.
func (d *DAG) AddRoot(root Vertex) {
	d.roots = append(d.roots, root)
}

// RemoveRoot removes the given root from the DAG's roots if it exists.
func (d *DAG) RemoveRoot(root Vertex) {
	idx := -1
	for i := range d.roots {
		if d.roots[i] == root {
			idx = i
			break
		}
	}

	if idx >= 0 {
		d.roots = append(d.roots[:idx], d.roots[idx+1:]...)
	}
}

type MatchCondition interface {
	fmt.Stringer
}

// PrefixMatchType represents different types of prefix matching alternatives.
type PrefixMatchType int

const (
	// PrefixMatchString represents a prefix match that functions like a
	// string prefix match, i.e. prefix /foo matches /foobar
	PrefixMatchString PrefixMatchType = iota
	// PrefixMatchSegment represents a prefix match that only matches full path
	// segments, i.e. prefix /foo matches /foo/bar but not /foobar
	PrefixMatchSegment
)

var prefixMatchTypeToName = map[PrefixMatchType]string{
	PrefixMatchString:  "string",
	PrefixMatchSegment: "segment",
}

// PrefixMatchCondition matches the start of a URL.
type PrefixMatchCondition struct {
	Prefix          string
	PrefixMatchType PrefixMatchType
}

func (ec *ExactMatchCondition) String() string {
	return "exact: " + ec.Path
}

// ExactMatchCondition matches the entire path of a URL.
type ExactMatchCondition struct {
	Path string
}

func (pc *PrefixMatchCondition) String() string {
	str := "prefix: " + pc.Prefix
	if typeStr, ok := prefixMatchTypeToName[pc.PrefixMatchType]; ok {
		str += " type: " + typeStr
	}
	return str
}

// RegexMatchCondition matches the URL by regular expression.
type RegexMatchCondition struct {
	Regex string
}

func (rc *RegexMatchCondition) String() string {
	return "regex: " + rc.Regex
}

const (
	// HeaderMatchTypeExact matches a header value exactly.
	HeaderMatchTypeExact = "exact"

	// HeaderMatchTypeContains matches a header value if it contains the
	// provided value.
	HeaderMatchTypeContains = "contains"

	// HeaderMatchTypePresent matches a header if it is present in a request.
	HeaderMatchTypePresent = "present"

	// HeaderMatchTypeRegex matches a header if it matches the provided regular
	// expression.
	HeaderMatchTypeRegex = "regex"
)

// HeaderMatchCondition matches request headers by MatchType
type HeaderMatchCondition struct {
	Name      string
	Value     string
	MatchType string
	Invert    bool
}

func (hc *HeaderMatchCondition) String() string {
	details := strings.Join([]string{
		"name=" + hc.Name,
		"value=" + hc.Value,
		"matchtype=", hc.MatchType,
		"invert=", strconv.FormatBool(hc.Invert),
	}, "&")

	return "header: " + details
}

// DirectResponse allows for a specific HTTP status code
// to be the response to a route request vs routing to
// an envoy cluster.
type DirectResponse struct {
	StatusCode uint32
}

// Route defines the properties of a route to a Cluster.
type Route struct {

	// PathMatchCondition specifies a MatchCondition to match on the request path.
	// Must not be nil.
	PathMatchCondition MatchCondition

	// HeaderMatchConditions specifies a set of additional Conditions to
	// match on the request headers.
	HeaderMatchConditions []HeaderMatchCondition

	Clusters []*Cluster

	// Should this route generate a 301 upgrade if accessed
	// over HTTP?
	HTTPSUpgrade bool

	// AuthDisabled is set if authorization should be disabled
	// for this route. If authorization is disabled, the AuthContext
	// field has no effect.
	AuthDisabled bool

	// AuthContext sets the authorization context (if authorization is enabled).
	AuthContext map[string]string

	// Is this a websocket route?
	// TODO(dfc) this should go on the service
	Websocket bool

	// TimeoutPolicy defines the timeout request/idle
	TimeoutPolicy TimeoutPolicy

	// RetryPolicy defines the retry / number / timeout options for a route
	RetryPolicy *RetryPolicy

	// Indicates that during forwarding, the matched prefix (or path) should be swapped with this value
	PrefixRewrite string

	// Mirror Policy defines the mirroring policy for this Route.
	MirrorPolicy *MirrorPolicy

	// RequestHeadersPolicy defines how headers are managed during forwarding
	RequestHeadersPolicy *HeadersPolicy

	// ResponseHeadersPolicy defines how headers are managed during forwarding
	ResponseHeadersPolicy *HeadersPolicy

	// RateLimitPolicy defines if/how requests for the route are rate limited.
	RateLimitPolicy *RateLimitPolicy

	// RequestHashPolicies is a list of policies for configuring hashes on
	// request attributes.
	RequestHashPolicies []RequestHashPolicy

	// DirectResponse allows for a specific HTTP status code
	// to be the response to a route request vs routing to
	// an envoy cluster.
	DirectResponse *DirectResponse
}

// HasPathPrefix returns whether this route has a PrefixPathCondition.
func (r *Route) HasPathPrefix() bool {
	_, ok := r.PathMatchCondition.(*PrefixMatchCondition)
	return ok
}

// HasPathRegex returns whether this route has a RegexPathCondition.
func (r *Route) HasPathRegex() bool {
	_, ok := r.PathMatchCondition.(*RegexMatchCondition)
	return ok
}

// TimeoutPolicy defines the timeout policy for a route.
type TimeoutPolicy struct {
	// ResponseTimeout is the timeout applied to the response
	// from the backend server.
	ResponseTimeout timeout.Setting

	// IdleTimeout is the timeout applied to idle connections.
	IdleTimeout timeout.Setting
}

// RetryPolicy defines the retry / number / timeout options
type RetryPolicy struct {
	// RetryOn specifies the conditions under which retry takes place.
	// If empty, retries will not be performed.
	RetryOn string

	// RetriableStatusCodes specifies the HTTP status codes under which retry takes place.
	RetriableStatusCodes []uint32

	// NumRetries specifies the allowed number of retries.
	// Ignored if RetryOn is blank, or defaults to 1 if RetryOn is set.
	NumRetries uint32

	// PerTryTimeout specifies the timeout per retry attempt.
	// Ignored if RetryOn is blank.
	PerTryTimeout timeout.Setting
}

// MirrorPolicy defines the mirroring policy for a route.
type MirrorPolicy struct {
	Cluster *Cluster
}

// HeadersPolicy defines how headers are managed during forwarding
type HeadersPolicy struct {
	// HostRewrite defines if a host should be rewritten on upstream requests
	HostRewrite string

	Add    map[string]string
	Set    map[string]string
	Remove []string
}

// RateLimitPolicy holds rate limiting parameters.
type RateLimitPolicy struct {
	Local  *LocalRateLimitPolicy
	Global *GlobalRateLimitPolicy
}

// LocalRateLimitPolicy holds local rate limiting parameters.
type LocalRateLimitPolicy struct {
	MaxTokens            uint32
	TokensPerFill        uint32
	FillInterval         time.Duration
	ResponseStatusCode   uint32
	ResponseHeadersToAdd map[string]string
}

// HeaderHashOptions contains options for hashing a HTTP header.
type HeaderHashOptions struct {
	// HeaderName is the name of the header to hash.
	HeaderName string
}

// CookieHashOptions contains options for hashing a HTTP cookie.
type CookieHashOptions struct {
	// CookieName is the name of the header to hash.
	CookieName string

	// TTL is how long the cookie should be valid for.
	TTL time.Duration

	// Path is the request path the cookie is valid for.
	Path string
}

// RequestHashPolicy holds configuration for calculating hashes on
// an individual request attribute.
type RequestHashPolicy struct {
	// Terminal determines if the request attribute is present, hash
	// calculation should stop with this element.
	Terminal bool

	// HeaderHashOptions is set when a header hash is desired.
	HeaderHashOptions *HeaderHashOptions

	// CookieHashOptions is set when a cookie hash is desired.
	CookieHashOptions *CookieHashOptions
}

// GlobalRateLimitPolicy holds global rate limiting parameters.
type GlobalRateLimitPolicy struct {
	Descriptors []*RateLimitDescriptor
}

// RateLimitDescriptor is a list of rate limit descriptor entries.
type RateLimitDescriptor struct {
	Entries []RateLimitDescriptorEntry
}

// RateLimitDescriptorEntry is an entry in a rate limit descriptor.
// Exactly one field should be non-nil.
type RateLimitDescriptorEntry struct {
	GenericKey       *GenericKeyDescriptorEntry
	HeaderMatch      *HeaderMatchDescriptorEntry
	HeaderValueMatch *HeaderValueMatchDescriptorEntry
	RemoteAddress    *RemoteAddressDescriptorEntry
}

// GenericKeyDescriptorEntry  configures a descriptor entry
// that has a static key & value.
type GenericKeyDescriptorEntry struct {
	Key   string
	Value string
}

// HeaderMatchDescriptorEntry configures a descriptor entry
// that's populated only if the specified header is present
// on the request.
type HeaderMatchDescriptorEntry struct {
	HeaderName string
	Key        string
}

type HeaderValueMatchDescriptorEntry struct {
	Headers     []HeaderMatchCondition
	ExpectMatch bool
	Value       string
}

// RemoteAddressDescriptorEntry configures a descriptor entry
// that contains the remote address (i.e. client IP).
type RemoteAddressDescriptorEntry struct{}

// CORSPolicy allows setting the CORS policy
type CORSPolicy struct {
	// Specifies whether the resource allows credentials.
	AllowCredentials bool
	// AllowOrigin specifies the origins that will be allowed to do CORS requests.
	AllowOrigin []string
	// AllowMethods specifies the content for the *access-control-allow-methods* header.
	AllowMethods []string
	// AllowHeaders specifies the content for the *access-control-allow-headers* header.
	AllowHeaders []string
	// ExposeHeaders Specifies the content for the *access-control-expose-headers* header.
	ExposeHeaders []string
	// MaxAge specifies the content for the *access-control-max-age* header.
	MaxAge timeout.Setting
}

type HeaderValue struct {
	// Name represents a key of a header
	Key string
	// Value represents the value of a header specified by a key
	Value string
}

// PeerValidationContext defines how to validate the certificate on the upstream service.
type PeerValidationContext struct {
	// CACertificate holds a reference to the Secret containing the CA to be used to
	// verify the upstream connection.
	CACertificate *Secret
	// SubjectName holds an optional subject name which Envoy will check against the
	// certificate presented by the upstream.
	SubjectName string
	// SkipClientCertValidation when set to true will ensure Envoy requests but
	// does not verify peer certificates.
	SkipClientCertValidation bool
}

// GetCACertificate returns the CA certificate from PeerValidationContext.
func (pvc *PeerValidationContext) GetCACertificate() []byte {
	if pvc == nil || pvc.CACertificate == nil {
		// No validation required.
		return nil
	}
	return pvc.CACertificate.Object.Data[CACertificateKey]
}

// GetSubjectName returns the SubjectName from PeerValidationContext.
func (pvc *PeerValidationContext) GetSubjectName() string {
	if pvc == nil {
		// No validation required.
		return ""
	}
	return pvc.SubjectName
}

func (r *Route) Visit(f func(Vertex)) {
	for _, c := range r.Clusters {
		f(c)
	}
	// Allow any mirror clusters to also be visited so that
	// they are also added to CDS.
	if r.MirrorPolicy != nil && r.MirrorPolicy.Cluster != nil {
		f(r.MirrorPolicy.Cluster)
	}
}

// A VirtualHost represents a named L4/L7 service.
type VirtualHost struct {
	// Name is the fully qualified domain name of a network host,
	// as defined by RFC 3986.
	Name string

	ListenerName string

	// CORSPolicy is the cross-origin policy to apply to the VirtualHost.
	CORSPolicy *CORSPolicy

	// RateLimitPolicy defines if/how requests for the virtual host
	// are rate limited.
	RateLimitPolicy *RateLimitPolicy

	routes map[string]*Route
}

func (v *VirtualHost) addRoute(route *Route) {
	if v.routes == nil {
		v.routes = make(map[string]*Route)
	}
	v.routes[conditionsToString(route)] = route
}

func conditionsToString(r *Route) string {
	s := []string{r.PathMatchCondition.String()}
	for _, cond := range r.HeaderMatchConditions {
		s = append(s, cond.String())
	}
	return strings.Join(s, ",")
}

func (v *VirtualHost) Visit(f func(Vertex)) {
	for _, r := range v.routes {
		f(r)
	}
}

func (v *VirtualHost) Valid() bool {
	// A VirtualHost is valid if it has at least one route.
	return len(v.routes) > 0
}

// A SecureVirtualHost represents a HTTP host protected by TLS.
type SecureVirtualHost struct {
	VirtualHost

	// TLS minimum protocol version. Defaults to envoy_tls_v3.TlsParameters_TLS_AUTO
	MinTLSVersion string

	// The cert and key for this host.
	Secrets map[string]*Secret

	// FallbackCertificate
	FallbackCertificate *Secret

	// Service to TCP proxy all incoming connections.
	*TCPProxy

	// DownstreamValidation defines how to verify the client's certificate.
	DownstreamValidation *PeerValidationContext

	// AuthorizationService points to the extension that client
	// requests are forwarded to for authorization. If nil, no
	// authorization is enabled for this host.
	AuthorizationService *ExtensionCluster

	// AuthorizationResponseTimeout sets how long the proxy should wait
	// for authorization server responses.
	AuthorizationResponseTimeout timeout.Setting

	// AuthorizationFailOpen sets whether authorization server
	// failures should cause the client request to also fail. The
	// only reason to set this to `true` is when you are migrating
	// from internal to external authorization.
	AuthorizationFailOpen bool
}

func (s *SecureVirtualHost) Visit(f func(Vertex)) {
	s.VirtualHost.Visit(f)
	if s.TCPProxy != nil {
		f(s.TCPProxy)
	}
	if len(s.Secrets) > 0 {
		for _, secret := range s.Secrets {
			f(secret) // secret is not required if vhost is using tls passthrough
		}
	}
}

func (s *SecureVirtualHost) Valid() bool {
	// A SecureVirtualHost is valid if either
	// 1. it has a secret and at least one route.
	// 2. it has a tcpproxy, because the tcpproxy backend may negotiate TLS itself.
	return (len(s.Secrets) > 0 && len(s.routes) > 0) || s.TCPProxy != nil
}

type ListenerName struct {
	Name         string
	ListenerName string
}

// A Listener represents a TCP socket that accepts
// incoming connections.
type Listener struct {

	// Address is the TCP address to listen on.
	// If blank 0.0.0.0, or ::/0 for IPv6, is assumed.
	Address string

	// Port is the TCP port to listen on.
	Port int

	VirtualHosts []Vertex
}

func (l *Listener) Visit(f func(Vertex)) {
	for _, vh := range l.VirtualHosts {
		f(vh)
	}
}

// TCPProxy represents a cluster of TCP endpoints.
type TCPProxy struct {

	// Clusters is the, possibly weighted, set
	// of upstream services to forward decrypted traffic.
	Clusters []*Cluster
}

func (t *TCPProxy) Visit(f func(Vertex)) {
	for _, s := range t.Clusters {
		f(s)
	}
}

// Service represents a single Kubernetes' Service's Port.
type Service struct {
	Weighted WeightedService

	// Protocol is the layer 7 protocol of this service
	// One of "", "h2", "h2c", or "tls".
	Protocol string

	// Circuit breaking limits

	// Max connections is maximum number of connections
	// that Envoy will make to the upstream cluster.
	MaxConnections uint32

	// MaxPendingRequests is maximum number of pending
	// requests that Envoy will allow to the upstream cluster.
	MaxPendingRequests uint32

	// MaxRequests is the maximum number of parallel requests that
	// Envoy will make to the upstream cluster.
	MaxRequests uint32

	// MaxRetries is the maximum number of parallel retries that
	// Envoy will allow to the upstream cluster.
	MaxRetries uint32

	// ExternalName is an optional field referencing a dns entry for Service type "ExternalName"
	ExternalName string
}

// Visit applies the visitor function to the Service vertex.
func (s *Service) Visit(f func(Vertex)) {
	// A Service has only one WeightedService entry. Fake up a
	// ServiceCluster so that the visitor can pretend to not
	// know this.
	c := ServiceCluster{
		ClusterName: xds.ClusterLoadAssignmentName(
			types.NamespacedName{
				Name:      s.Weighted.ServiceName,
				Namespace: s.Weighted.ServiceNamespace,
			},
			s.Weighted.ServicePort.Name),
		Services: []WeightedService{
			s.Weighted,
		},
	}

	f(&c)
}

// Cluster holds the connection specific parameters that apply to
// traffic routed to an upstream service.
type Cluster struct {
	// Upstream is the backend Kubernetes service traffic arriving
	// at this Cluster will be forwarded too.
	Upstream *Service

	// The relative weight of this Cluster compared to its siblings.
	Weight uint32

	// The protocol to use to speak to this cluster.
	Protocol string

	// UpstreamValidation defines how to verify the backend service's certificate
	UpstreamValidation *PeerValidationContext

	// The load balancer strategy to use when picking a host in the cluster.
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#enum-config-cluster-v3-cluster-lbpolicy
	LoadBalancerPolicy string

	// Cluster http health check policy
	*HTTPHealthCheckPolicy

	// Cluster tcp health check policy
	*TCPHealthCheckPolicy

	// RequestHeadersPolicy defines how headers are managed during forwarding
	RequestHeadersPolicy *HeadersPolicy

	// ResponseHeadersPolicy defines how headers are managed during forwarding
	ResponseHeadersPolicy *HeadersPolicy

	// SNI is used when a route proxies an upstream using tls.
	// SNI describes how the SNI is set on a Cluster and is configured via RequestHeadersPolicy.Host key.
	// Policies set on service are used before policies set on a route. Otherwise the value of the externalService
	// is used if the route is configured to proxy to an externalService type.
	// If the value is not set, then SNI is not changed.
	SNI string

	// DNSLookupFamily defines how external names are looked up
	// When configured as V4, the DNS resolver will only perform a lookup
	// for addresses in the IPv4 family. If V6 is configured, the DNS resolver
	// will only perform a lookup for addresses in the IPv6 family.
	// If AUTO is configured, the DNS resolver will first perform a lookup
	// for addresses in the IPv6 family and fallback to a lookup for addresses
	// in the IPv4 family.
	// Note: This only applies to externalName clusters.
	DNSLookupFamily string

	// ClientCertificate is the optional identifier of the TLS secret containing client certificate and
	// private key to be used when establishing TLS connection to upstream cluster.
	ClientCertificate *Secret
}

func (c Cluster) Visit(f func(Vertex)) {
	f(c.Upstream)
}

// WeightedService represents the load balancing weight of a
// particular v1.Weighted port.
type WeightedService struct {
	// Weight is the integral load balancing weight.
	Weight uint32
	// ServiceName is the v1.Service name.
	ServiceName string
	// ServiceNamespace is the v1.Service namespace.
	ServiceNamespace string
	// ServicePort is the port to which we forward traffic.
	ServicePort v1.ServicePort
}

// ServiceCluster capture the set of Kubernetes Services that will
// compose the endpoints for a Envoy cluster. Traffic is balanced
// across the Service slice based on the weight of the elements.
type ServiceCluster struct {
	// ClusterName is a globally unique name for this ServiceCluster.
	// It is eventually used as the Envoy ClusterLoadAssignment
	// name, and must not be empty.
	ClusterName string
	// Services are the load balancing targets. This slice must not be empty.
	Services []WeightedService
}

// DeepCopy performs a deep copy of ServiceClusters
// TODO(jpeach): apply deepcopy-gen to DAG objects.
func (s *ServiceCluster) DeepCopy() *ServiceCluster {
	s2 := ServiceCluster{
		ClusterName: s.ClusterName,
		Services:    make([]WeightedService, len(s.Services)),
	}

	for i, w := range s.Services {
		s2.Services[i] = w
		w.ServicePort.DeepCopyInto(&s2.Services[i].ServicePort)
	}

	return &s2
}

// Validate checks whether this ServiceCluster satisfies its semantic invariants.
func (s *ServiceCluster) Validate() error {
	if s.ClusterName == "" {
		return errors.New("missing .ClusterName field")
	}

	if len(s.Services) == 0 {
		return errors.New("empty .Services field")
	}

	for i, w := range s.Services {
		if w.ServiceName == "" {
			return fmt.Errorf("empty .Services[%d].ServiceName field", i)
		}

		if w.ServiceNamespace == "" {
			return fmt.Errorf("empty .Services[%d].ServiceNamespace field", i)
		}
	}

	return nil
}

func (s *ServiceCluster) Visit(func(Vertex)) {
	// ServiceClusters are leaves in the DAG.
}

// AddService adds the given service with a default weight of 1.
func (s *ServiceCluster) AddService(name types.NamespacedName, port v1.ServicePort) {
	s.AddWeightedService(1, name, port)
}

// AddWeightedService adds the given service with the given weight.
func (s *ServiceCluster) AddWeightedService(weight uint32, name types.NamespacedName, port v1.ServicePort) {
	w := WeightedService{
		Weight:           weight,
		ServiceName:      name.Name,
		ServiceNamespace: name.Namespace,
		ServicePort:      port,
	}

	s.Services = append(s.Services, w)
}

// Rebalance rewrites the weights for the service cluster so that
// if no weights are specifies, the traffic is evenly distributed.
// This matches the behavior of weighted routes. Note that this is
// a destructive operation.
func (s *ServiceCluster) Rebalance() {
	var sum uint32

	for _, w := range s.Services {
		sum += w.Weight
	}

	if sum == 0 {
		for i := range s.Services {
			s.Services[i].Weight = 1
		}
	}
}

// Secret represents a K8s Secret for TLS usage as a DAG Vertex. A Secret is
// a leaf in the DAG.
type Secret struct {
	Object *v1.Secret
}

func (s *Secret) Name() string       { return s.Object.Name }
func (s *Secret) Namespace() string  { return s.Object.Namespace }
func (s *Secret) Visit(func(Vertex)) {}

// Data returns the contents of the backing secret's map.
func (s *Secret) Data() map[string][]byte {
	return s.Object.Data
}

// Cert returns the secret's tls certificate
func (s *Secret) Cert() []byte {
	return s.Object.Data[v1.TLSCertKey]
}

// PrivateKey returns the secret's tls private key
func (s *Secret) PrivateKey() []byte {
	return s.Object.Data[v1.TLSPrivateKeyKey]
}

// HTTPHealthCheckPolicy http health check policy
type HTTPHealthCheckPolicy struct {
	Path               string
	Host               string
	Interval           time.Duration
	Timeout            time.Duration
	UnhealthyThreshold uint32
	HealthyThreshold   uint32
}

// TCPHealthCheckPolicy tcp health check policy
type TCPHealthCheckPolicy struct {
	Interval           time.Duration
	Timeout            time.Duration
	UnhealthyThreshold uint32
	HealthyThreshold   uint32
}

// ExtensionCluster generates an Envoy cluster (aka ClusterLoadAssignment)
// for an ExtensionService resource.
type ExtensionCluster struct {
	// Name is the (globally unique) name of the corresponding Envoy cluster resource.
	Name string

	// Upstream is the cluster that receives network traffic.
	Upstream ServiceCluster

	// The protocol to use to speak to this cluster.
	Protocol string

	// UpstreamValidation defines how to verify the backend service's certificate
	UpstreamValidation *PeerValidationContext

	// The load balancer type to use when picking a host in the cluster.
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#enum-config-cluster-v3-cluster-lbpolicy
	LoadBalancerPolicy string

	// TimeoutPolicy specifies how to handle timeouts to this extension.
	TimeoutPolicy TimeoutPolicy

	// SNI is used when a route proxies an upstream using TLS.
	SNI string

	// ClientCertificate is the optional identifier of the TLS secret containing client certificate and
	// private key to be used when establishing TLS connection to upstream cluster.
	ClientCertificate *Secret
}

// Visit processes extension clusters.
func (e *ExtensionCluster) Visit(f func(Vertex)) {
	// Emit the upstream ServiceCluster to the visitor.
	f(&e.Upstream)
}
