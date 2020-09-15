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

package contour

import (
	"testing"

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHTTPProxyMetrics(t *testing.T) {
	type testcase struct {
		objs           []interface{}
		wantIR         *metrics.RouteMetric
		wantProxy      *metrics.RouteMetric
		rootNamespaces []string
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			builder := dag.Builder{
				Source: dag.KubernetesCache{
					RootNamespaces: tc.rootNamespaces,
					FieldLogger:    fixture.NewTestLogger(t),
				},
				Processors: []dag.Processor{
					&dag.IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&dag.HTTPProxyProcessor{},
					&dag.ListenerProcessor{},
				},
			}
			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}

			dag := builder.Build()

			gotProxy := calculateRouteMetric(dag.Statuses())

			if tc.wantProxy != nil {
				assert.Equal(t, *tc.wantProxy, gotProxy)
			}
		})
	}

	// proxy1 is a valid httpproxy
	proxy1 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy2 is invalid because it contains a service with negative port
	proxy2 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: -80,
				}},
			}},
		},
	}

	// proxy3 is invalid because it lives outside the roots namespace
	proxy3 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "example",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foobar",
				}},
				Services: []projcontour.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	//// proxy4 is invalid because its match prefix does not match its parent's (proxy1)
	//proxy4 := &projcontour.HTTPProxy{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Namespace: "roots",
	//		Name:      "delegated",
	//	},
	//	Spec: projcontour.HTTPProxySpec{
	//		Routes: []projcontour.Route{{
	//			Conditions: []projcontour.MatchCondition{{
	//				Prefix: "/doesnotmatch",
	//			}},
	//			Services: []projcontour.Service{{
	//				Name: "home",
	//				Port: 8080,
	//			}},
	//		}},
	//	},
	//}

	// proxy6 is invalid because it delegates to itself, producing a cycle
	proxy6 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "self",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "self",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	// proxy7 delegates to proxy8, which is invalid because it delegates back to proxy7
	proxy7 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "child",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxy8 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "child",
		},
		Spec: projcontour.HTTPProxySpec{
			Includes: []projcontour.Include{{
				Name: "parent",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	//// proxy9 is invalid because it has a route that both delegates and has a list of services
	//proxy9 := &projcontour.HTTPProxy{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Namespace: "roots",
	//		Name:      "parent",
	//	},
	//	Spec: projcontour.HTTPProxySpec{
	//		VirtualHost: &projcontour.VirtualHost{
	//			Fqdn: "example.com",
	//		},
	//		Includes: []projcontour.Include{{
	//			Name: "child",
	//			Conditions: []projcontour.MatchCondition{{
	//				Prefix: "/foo",
	//			}},
	//		}},
	//		Routes: []projcontour.Route{{
	//			Services: []projcontour.Service{{
	//				Name: "kuard",
	//				Port: 8080,
	//			}},
	//		}},
	//	},
	//}

	// proxy10 delegates to proxy11 and proxy12.
	proxy10 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []projcontour.Include{{
				Name: "validChild",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
			}, {
				Name: "invalidChild",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	proxy11 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "validChild",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy12 is invalid because it contains an invalid port
	proxy12 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidChild",
		},
		Spec: projcontour.HTTPProxySpec{
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/bar",
				}},
				Services: []projcontour.Service{{
					Name: "foo",
					Port: 12345678,
				}},
			}},
		},
	}

	// proxy13 is invalid because it does not specify and FQDN
	proxy13 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{},
			Routes: []projcontour.Route{{
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []projcontour.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	proxy14 := &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidParent",
		},
		Spec: projcontour.HTTPProxySpec{
			VirtualHost: &projcontour.VirtualHost{},
			Includes: []projcontour.Include{{
				Name: "validChild",
				Conditions: []projcontour.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "foo",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     12345678,
			}},
		},
	}

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "foo",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	s3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "home",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	run(t, "valid proxy", testcase{
		objs:   []interface{}{proxy1, s3},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{},
			Valid: map[metrics.Meta]int{
				{Namespace: "roots", VHost: "example.com"}: 1,
			},
			Orphaned: map[metrics.Meta]int{},
			Root: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Total: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
		},
	})

	run(t, "invalid port in service - proxy", testcase{
		objs:   []interface{}{proxy2},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{
				{Namespace: "roots", VHost: "example.com"}: 1,
			},
			Valid:    map[metrics.Meta]int{},
			Orphaned: map[metrics.Meta]int{},
			Root: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Total: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
		},
	})

	run(t, "root proxy outside of roots namespace", testcase{
		objs:   []interface{}{proxy3},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{
				{Namespace: "finance"}: 1,
			},
			Valid:    map[metrics.Meta]int{},
			Orphaned: map[metrics.Meta]int{},
			Root: map[metrics.Meta]int{
				{Namespace: "finance"}: 1,
			},
			Total: map[metrics.Meta]int{
				{Namespace: "finance"}: 1,
			},
		},
		rootNamespaces: []string{"foo"},
	})

	run(t, "root proxy does not specify FQDN", testcase{
		objs:   []interface{}{proxy13},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Valid:    map[metrics.Meta]int{},
			Orphaned: map[metrics.Meta]int{},
			Root: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Total: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
		},
	})

	run(t, "self-edge produces a cycle - proxy", testcase{
		objs:   []interface{}{proxy6},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{
				{Namespace: "roots", VHost: "example.com"}: 1,
			},
			Valid:    map[metrics.Meta]int{},
			Orphaned: map[metrics.Meta]int{},
			Root: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Total: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
		},
	})

	run(t, "child delegates to parent, producing a cycle - proxy", testcase{
		objs:   []interface{}{proxy7, proxy8},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Valid: map[metrics.Meta]int{
				{Namespace: "roots", VHost: "example.com"}: 1,
			},
			Orphaned: map[metrics.Meta]int{},
			Root: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Total: map[metrics.Meta]int{
				{Namespace: "roots"}: 2,
			},
		},
	})

	run(t, "proxy is an orphaned route", testcase{
		objs:   []interface{}{proxy8},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{},
			Valid:   map[metrics.Meta]int{},
			Orphaned: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Root: map[metrics.Meta]int{},
			Total: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
		},
	})

	run(t, "proxy delegates to multiple proxies, one is invalid", testcase{
		objs:   []interface{}{proxy10, proxy11, proxy12, s1, s2},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Valid: map[metrics.Meta]int{
				{Namespace: "roots"}:                       1,
				{Namespace: "roots", VHost: "example.com"}: 1,
			},
			Orphaned: map[metrics.Meta]int{},
			Root: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Total: map[metrics.Meta]int{
				{Namespace: "roots"}: 3,
			},
		},
	})

	run(t, "invalid parent orphans children - proxy", testcase{
		objs:   []interface{}{proxy14, proxy11},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Valid: map[metrics.Meta]int{},
			Orphaned: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Root: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Total: map[metrics.Meta]int{
				{Namespace: "roots"}: 2,
			},
		},
	})

	run(t, "multi-parent children is not orphaned when one of the parents is invalid - proxy", testcase{
		objs:   []interface{}{proxy14, proxy11, proxy10, s2},
		wantIR: nil,
		wantProxy: &metrics.RouteMetric{
			Invalid: map[metrics.Meta]int{
				{Namespace: "roots"}:                       1,
				{Namespace: "roots", VHost: "example.com"}: 1,
			},
			Valid: map[metrics.Meta]int{
				{Namespace: "roots"}: 1,
			},
			Orphaned: map[metrics.Meta]int{},
			Root: map[metrics.Meta]int{
				{Namespace: "roots"}: 2,
			},
			Total: map[metrics.Meta]int{
				{Namespace: "roots"}: 3,
			},
		},
	})
}
