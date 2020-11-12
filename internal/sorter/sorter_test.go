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

package sorter

import (
	"math/rand"
	"sort"
	"testing"

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/stretchr/testify/assert"
)

func shuffleRoutes(routes []*envoy_route_v3.Route) []*envoy_route_v3.Route {
	shuffled := make([]*envoy_route_v3.Route, len(routes))

	copy(shuffled, routes)

	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled
}

func TestInvalidSorter(t *testing.T) {
	assert.Equal(t, nil, For([]string{"invalid"}))
}

func TestSortRouteConfiguration(t *testing.T) {
	want := []*envoy_route_v3.RouteConfiguration{
		{Name: "bar"},
		{Name: "baz"},
		{Name: "foo"},
		{Name: "same", InternalOnlyHeaders: []string{"z", "y"}},
		{Name: "same", InternalOnlyHeaders: []string{"a", "b"}},
	}

	have := []*envoy_route_v3.RouteConfiguration{
		want[3], // Ensure the "same" element stays stable.
		want[4],
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortVirtualHosts(t *testing.T) {
	want := []*envoy_route_v3.VirtualHost{
		{Name: "bar"},
		{Name: "baz"},
		{Name: "foo"},
		{Name: "same", Domains: []string{"z", "y"}},
		{Name: "same", Domains: []string{"a", "b"}},
	}

	have := []*envoy_route_v3.VirtualHost{
		want[3], // Ensure the "same" element stays stable.
		want[4],
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func matchPrefix(str string) *envoy_route_v3.RouteMatch_Prefix {
	return &envoy_route_v3.RouteMatch_Prefix{
		Prefix: str,
	}
}

func matchRegex(str string) *envoy_route_v3.RouteMatch_SafeRegex {
	return &envoy_route_v3.RouteMatch_SafeRegex{
		SafeRegex: &matcher.RegexMatcher{
			Regex: str,
		},
	}
}

func exactHeader(name string, value string) *envoy_route_v3.HeaderMatcher {
	return &envoy_route_v3.HeaderMatcher{
		Name: name,
		HeaderMatchSpecifier: &envoy_route_v3.HeaderMatcher_ExactMatch{
			ExactMatch: value,
		},
	}
}

func presentHeader(name string) *envoy_route_v3.HeaderMatcher {
	return &envoy_route_v3.HeaderMatcher{
		Name: name,
		HeaderMatchSpecifier: &envoy_route_v3.HeaderMatcher_PresentMatch{
			PresentMatch: true,
		},
	}
}

func TestSortRoutesLongestPath(t *testing.T) {
	want := []*envoy_route_v3.Route{
		{
			Match: &envoy_route_v3.RouteMatch{
				PathSpecifier: matchRegex("/this/is/the/longest"),
			}},

		// Note that regex matches sort before prefix matches.
		{
			Match: &envoy_route_v3.RouteMatch{
				PathSpecifier: matchRegex("."),
			}},

		{
			Match: &envoy_route_v3.RouteMatch{
				PathSpecifier: matchPrefix("/path/prefix2"),
			}},

		{
			Match: &envoy_route_v3.RouteMatch{
				PathSpecifier: matchPrefix("/path/prefix"),
			}},
	}

	have := shuffleRoutes(want)

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortRoutesLongestHeaders(t *testing.T) {
	want := []*envoy_route_v3.Route{
		{
			// Although the header names are the same, this value
			// should sort before the next one because it is
			// textually longer.
			Match: &envoy_route_v3.RouteMatch{
				PathSpecifier: matchPrefix("/path"),
				Headers: []*envoy_route_v3.HeaderMatcher{
					exactHeader("header-name", "header-value"),
				},
			},
		}, {
			Match: &envoy_route_v3.RouteMatch{
				PathSpecifier: matchPrefix("/path"),
				Headers: []*envoy_route_v3.HeaderMatcher{
					presentHeader("header-name"),
				},
			},
		}, {
			Match: &envoy_route_v3.RouteMatch{
				PathSpecifier: matchPrefix("/path"),
				Headers: []*envoy_route_v3.HeaderMatcher{
					exactHeader("long-header-name", "long-header-value"),
				},
			},
		}, {
			Match: &envoy_route_v3.RouteMatch{
				PathSpecifier: matchPrefix("/path"),
			},
		},
	}

	have := shuffleRoutes(want)

	sort.Stable(For(have))
	assert.Equal(t, want, have)
}

func TestSortSecrets(t *testing.T) {
	want := []*envoy_tls_v3.Secret{
		{Name: "first"},
		{Name: "second"},
	}

	have := []*envoy_tls_v3.Secret{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortHeaderMatchers(t *testing.T) {
	want := []*envoy_route_v3.HeaderMatcher{
		// Note that if the header names are the same, we
		// order by the protobuf string, in which case "exact"
		// is less than "present".
		exactHeader("header-name", "anything"),
		presentHeader("header-name"),
		exactHeader("long-header-name", "long-header-value"),
	}

	have := []*envoy_route_v3.HeaderMatcher{
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortClusters(t *testing.T) {
	want := []*envoy_cluster_v3.Cluster{
		{Name: "first"},
		{Name: "second"},
	}

	have := []*envoy_cluster_v3.Cluster{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortClusterLoadAssignments(t *testing.T) {
	want := []*envoy_endpoint_v3.ClusterLoadAssignment{
		{ClusterName: "first"},
		{ClusterName: "second"},
	}

	have := []*envoy_endpoint_v3.ClusterLoadAssignment{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortHTTPWeightedClusters(t *testing.T) {
	want := []*envoy_route_v3.WeightedCluster_ClusterWeight{
		{
			Name:   "first",
			Weight: protobuf.UInt32(10),
		},
		{
			Name:   "second",
			Weight: protobuf.UInt32(10),
		},
		{
			Name:   "second",
			Weight: protobuf.UInt32(20),
		},
	}

	have := []*envoy_route_v3.WeightedCluster_ClusterWeight{
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortTCPWeightedClusters(t *testing.T) {
	want := []*tcp.TcpProxy_WeightedCluster_ClusterWeight{
		{
			Name:   "first",
			Weight: 10,
		},
		{
			Name:   "second",
			Weight: 10,
		},
		{
			Name:   "second",
			Weight: 20,
		},
	}

	have := []*tcp.TcpProxy_WeightedCluster_ClusterWeight{
		want[2],
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortListeners(t *testing.T) {
	want := []*envoy_listener_v3.Listener{
		{Name: "first"},
		{Name: "second"},
	}

	have := []*envoy_listener_v3.Listener{
		want[1],
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}

func TestSortFilterChains(t *testing.T) {
	names := func(n ...string) *envoy_listener_v3.FilterChainMatch {
		return &envoy_listener_v3.FilterChainMatch{
			ServerNames: n,
		}
	}

	want := []*envoy_listener_v3.FilterChain{
		{
			FilterChainMatch: names("first"),
		},

		// The following two entries should match the order
		// in "have" because we are doing a stable sort, and
		// they are equal since we only compare the first
		// server name.
		{
			FilterChainMatch: names("second", "zzzzz"),
		},
		{
			FilterChainMatch: names("second", "aaaaa"),
		},
		{
			FilterChainMatch: &envoy_listener_v3.FilterChainMatch{},
		},
	}

	have := []*envoy_listener_v3.FilterChain{
		want[1], // zzzzz
		want[3], // blank
		want[2], // aaaaa
		want[0],
	}

	sort.Stable(For(have))
	assert.Equal(t, have, want)
}
