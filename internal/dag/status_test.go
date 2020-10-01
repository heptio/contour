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

package dag

import (
	"testing"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDAGStatus(t *testing.T) {

	type testcase struct {
		objs                []interface{}
		fallbackCertificate *types.NamespacedName
		want                map[types.NamespacedName]contour_api_v1.DetailedCondition
	}

	run := func(t *testing.T, desc string, tc testcase) {
		t.Helper()
		t.Run(desc, func(t *testing.T) {
			t.Helper()
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: []string{"roots", "marketing"},
					FieldLogger:    fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{
						FallbackCertificate: tc.fallbackCertificate,
					},
					&ListenerProcessor{},
				},
			}
			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()
			t.Logf("%#v\n", dag.StatusCache)
			got := dag.Statuses()
			assert.Equal(t, tc.want, got)
		})
	}

	// Common test fixtures (used across more than one test)

	// proxyNoFQDN is invalid because it does not specify and FQDN
	proxyNoFQDN := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "parent",
			Generation: 23,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "foo",
					Port: 8080,
				}},
			}},
		},
	}

	// Tests using common fixtures
	run(t, "root proxy does not specify FQDN", testcase{
		objs: []interface{}{proxyNoFQDN},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: fixture.NewValidCondition().WithGeneration(proxyNoFQDN.Generation).
				WithError("VirtualHostError", "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
		},
	})

	// Simple Valid HTTPProxy
	proxyValidHomeService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "example",
			Generation: 24,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "valid proxy", testcase{
		objs: []interface{}{proxyValidHomeService, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidHomeService.Name, Namespace: proxyValidHomeService.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidHomeService.Generation).
				Valid(),
		},
	})

	// Multiple Includes, one invalid
	proxyMultiIncludeOneInvalid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "parent",
			Generation: 45,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "validChild",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}, {
				Name: "invalidChild",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/bar",
				}},
			}},
		},
	}

	proxyIncludeValidChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "parentvalidchild",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "validChild",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxyChildValidFoo2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "roots",
			Name:       "validChild",
			Generation: 1,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	proxyChildInvalidBadPort := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidChild",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "foo3",
					Port: 12345678,
				}},
			}},
		},
	}

	run(t, "proxy has multiple includes, one is invalid", testcase{
		objs: []interface{}{proxyMultiIncludeOneInvalid, proxyChildValidFoo2, proxyChildInvalidBadPort, fixture.ServiceRootsFoo2, fixture.ServiceRootsFoo3InvalidPort},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyChildValidFoo2.Generation).
				Valid(),
			{Name: proxyChildInvalidBadPort.Name, Namespace: proxyChildInvalidBadPort.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyChildInvalidBadPort.Generation).
				WithError("ServiceError", "ServicePortInvalid", `service "foo3": port must be in the range 1-65535`),
			{Name: proxyMultiIncludeOneInvalid.Name, Namespace: proxyMultiIncludeOneInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyMultiIncludeOneInvalid.Generation).
				Valid(),
		},
	})

	run(t, "multi-parent child is not orphaned when one of the parents is invalid", testcase{
		objs: []interface{}{proxyNoFQDN, proxyChildValidFoo2, proxyIncludeValidChild, fixture.ServiceRootsKuard, fixture.ServiceRootsFoo2},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyNoFQDN.Name, Namespace: proxyNoFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyNoFQDN.Generation).
				WithError("VirtualHostError", "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
			{Name: proxyChildValidFoo2.Name, Namespace: proxyChildValidFoo2.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyChildValidFoo2.Generation).
				Valid(),
			{Name: proxyIncludeValidChild.Name, Namespace: proxyIncludeValidChild.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludeValidChild.Generation).
				Valid(),
		},
	})

	ingressSharedService := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: fixture.ServiceRootsNginx.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: fixture.SecretRootsCert.Name,
			}},
			Rules: []v1beta1.IngressRule{{
				Host:             "example.com",
				IngressRuleValue: ingressrulevalue(backend(fixture.ServiceRootsNginx.Name, intstr.FromInt(80))),
			}},
		},
	}

	proxyTCPSharedService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: fixture.ServiceRootsNginx.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsNginx.Name,
					Port: 80,
				}},
			},
		},
	}

	// issue 1399
	run(t, "service shared across ingress and httpproxy tcpproxy", testcase{
		objs: []interface{}{
			fixture.SecretRootsCert, fixture.ServiceRootsNginx, ingressSharedService, proxyTCPSharedService,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPSharedService.Name, Namespace: proxyTCPSharedService.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyTCPSharedService.Generation).
				Valid(),
		},
	})

	proxyDelegatedTCPTLS := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretProjectContourCert.Namespace + "/" + fixture.SecretProjectContourCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			},
		},
	}

	// issue 1347
	run(t, "tcpproxy with tls delegation failure", testcase{
		objs: []interface{}{
			fixture.SecretProjectContourCert,
			proxyDelegatedTCPTLS,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyDelegatedTCPTLS.Name, Namespace: proxyDelegatedTCPTLS.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyDelegatedTCPTLS.Generation).
				WithError("TLSError", "DelegationNotPermitted", `Spec.VirtualHost.TLS Secret "projectcontour/default-ssl-cert" certificate delegation not permitted`),
		},
	})

	proxyDelegatedTLS := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretProjectContourCert.Namespace + "/" + fixture.SecretProjectContourCert.Name,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "sample-app",
					Port: 80,
				}},
			}},
		},
	}

	// issue 1348
	run(t, "routes with tls delegation failure", testcase{
		objs: []interface{}{
			fixture.SecretProjectContourCert,
			proxyDelegatedTLS,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyDelegatedTLS.Name, Namespace: proxyDelegatedTLS.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyDelegatedTCPTLS.Generation).
				WithError("TLSError", "DelegationNotPermitted", `Spec.VirtualHost.TLS Secret "projectcontour/default-ssl-cert" certificate delegation not permitted`),
		},
	})

	serviceTLSPassthrough := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-passthrough",
			Namespace: "roots",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "https",
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromInt(443),
			}, {
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(80),
			}},
		},
	}

	proxyPassthroughProxyNonSecure := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: serviceTLSPassthrough.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: serviceTLSPassthrough.Name,
					Port: 80, // proxy non secure traffic to port 80
				}},
			}},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: serviceTLSPassthrough.Name,
					Port: 443, // ssl passthrough to secure port
				}},
			},
		},
	}

	// issue 910
	run(t, "non tls routes can be combined with tcp proxy", testcase{
		objs: []interface{}{
			serviceTLSPassthrough,
			proxyPassthroughProxyNonSecure,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyPassthroughProxyNonSecure.Name, Namespace: proxyPassthroughProxyNonSecure.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyPassthroughProxyNonSecure.Generation).
				Valid(),
		},
	})

	proxyMultipleIncludersSite1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site1",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "site1.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "www",
				Namespace: fixture.ServiceRootsKuard.Namespace,
			}},
		},
	}

	proxyMultipleIncludersSite2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site2",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "site2.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "www",
				Namespace: fixture.ServiceRootsKuard.Namespace,
			}},
		},
	}

	proxyMultiIncludeChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "two root httpproxies with different hostnames delegated to the same object are valid", testcase{
		objs: []interface{}{
			fixture.ServiceRootsKuard, proxyMultipleIncludersSite1, proxyMultipleIncludersSite2, proxyMultiIncludeChild,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyMultipleIncludersSite1.Name, Namespace: proxyMultipleIncludersSite1.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyMultipleIncludersSite1.Generation).
				Valid(),
			{Name: proxyMultipleIncludersSite2.Name, Namespace: proxyMultipleIncludersSite2.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyMultipleIncludersSite2.Generation).
				Valid(),
			{Name: proxyMultiIncludeChild.Name, Namespace: proxyMultiIncludeChild.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyMultiIncludeChild.Generation).
				Valid(),
		},
	})

	// proxyInvalidNegativePortHomeService is invalid because it contains a service with negative port
	proxyInvalidNegativePortHomeService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: -80,
				}},
			}},
		},
	}

	run(t, "invalid port in service", testcase{
		objs: []interface{}{proxyInvalidNegativePortHomeService},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidNegativePortHomeService.Name, Namespace: proxyInvalidNegativePortHomeService.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidNegativePortHomeService.Generation).
				WithError("ServiceError", "ServicePortInvalid", `service "home": port must be in the range 1-65535`),
		},
	})

	// proxyInvalidOutsideRootNamespace is invalid because it lives outside the roots namespace
	proxyInvalidOutsideRootNamespace := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foobar",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "root proxy outside of roots namespace", testcase{
		objs: []interface{}{proxyInvalidOutsideRootNamespace},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidOutsideRootNamespace.Name, Namespace: proxyInvalidOutsideRootNamespace.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidNegativePortHomeService.Generation).
				WithError("RootNamespaceError", "RootProxyNotAllowedInNamespace", "root HTTPProxy cannot be defined in this namespace"),
		},
	})

	// proxyInvalidIncludeCycle is invalid because it delegates to itself, producing a cycle
	proxyInvalidIncludeCycle := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "self",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "self",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "green",
					Port: 80,
				}},
			}},
		},
	}

	run(t, "proxy self-edge produces a cycle", testcase{
		objs: []interface{}{proxyInvalidIncludeCycle, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidIncludeCycle.Name, Namespace: proxyInvalidIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidIncludeCycle.Generation).
				WithError("IncludeError", "RootIncludesRoot", "root httpproxy cannot include another root httpproxy"),
		},
	})

	// proxyIncludesProxyWithIncludeCycle delegates to proxy8, which is invalid because proxy8 delegates back to proxy8
	proxyIncludesProxyWithIncludeCycle := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "child",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	proxyIncludedChildInvalidIncludeCycle := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Includes: []contour_api_v1.Include{{
				Name:      "child",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	run(t, "proxy child delegates to itself, producing a cycle", testcase{
		objs: []interface{}{proxyIncludesProxyWithIncludeCycle, proxyIncludedChildInvalidIncludeCycle},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyIncludesProxyWithIncludeCycle.Name, Namespace: proxyIncludesProxyWithIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludesProxyWithIncludeCycle.Generation).Valid(),
			{Name: proxyIncludedChildInvalidIncludeCycle.Name, Namespace: proxyIncludedChildInvalidIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludedChildInvalidIncludeCycle.Generation).
				WithError("IncludeError", "IncludeCreatesCycle", "include creates an include cycle: roots/parent -> roots/child -> roots/child"),
		},
	})

	run(t, "proxy orphaned route", testcase{
		objs: []interface{}{proxyIncludedChildInvalidIncludeCycle},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyIncludedChildInvalidIncludeCycle.Name, Namespace: proxyIncludedChildInvalidIncludeCycle.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludedChildInvalidIncludeCycle.Generation).
				Orphaned(),
		},
	})

	proxyIncludedChildValid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validChild",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "foo2",
					Port: 8080,
				}},
			}},
		},
	}

	// proxyNotRootIncludeRootProxy delegates to proxyWildCardFQDN but it is invalid because it is missing fqdn
	proxyNotRootIncludeRootProxy := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidParent",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{},
			Includes: []contour_api_v1.Include{{
				Name:      "validChild",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
			}},
		},
	}

	run(t, "proxy invalid parent orphans child", testcase{
		objs: []interface{}{proxyNotRootIncludeRootProxy, proxyIncludedChildValid},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyNotRootIncludeRootProxy.Name, Namespace: proxyNotRootIncludeRootProxy.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyNotRootIncludeRootProxy.Generation).
				WithError("VirtualHostError", "FQDNNotSpecified", "Spec.VirtualHost.Fqdn must be specified"),
			{Name: proxyIncludedChildValid.Name, Namespace: proxyIncludedChildValid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludedChildValid.Generation).
				Orphaned(),
		},
	})

	// proxyWildCardFQDN is invalid because it contains a wildcarded fqdn
	proxyWildCardFQDN := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.*.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy invalid FQDN contains wildcard", testcase{
		objs: []interface{}{proxyWildCardFQDN},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyWildCardFQDN.Name, Namespace: proxyWildCardFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyWildCardFQDN.Generation).
				WithError("VirtualHostError", "WildCardNotAllowed", `Spec.VirtualHost.Fqdn "example.*.com" cannot use wildcards`),
		},
	})

	// proxyInvalidServiceInvalid is invalid because it references an invalid service
	proxyInvalidServiceInvalid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "invalid",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy missing service is invalid", testcase{
		objs: []interface{}{proxyInvalidServiceInvalid},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidServiceInvalid.Name, Namespace: proxyInvalidServiceInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidServiceInvalid.Generation).
				WithError("ServiceError", "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "roots/invalid" not found`),
			// {
			// 	Object:      proxyInvalidServiceInvalid,
			// 	Status:      "invalid",
			// 	Description: `Spec.Routes unresolved service reference: service "roots/invalid" not found`,
			// 	Vhost:       proxyInvalidServiceInvalid.Spec.VirtualHost.Fqdn,
			// },
		},
	})

	// proxyInvalidServicePortInvalid is invalid because it references an invalid port on a service
	proxyInvalidServicePortInvalid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "invalidir",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 9999,
				}},
			}},
		},
	}

	run(t, "proxy with service missing port is invalid", testcase{
		objs: []interface{}{proxyInvalidServicePortInvalid, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidServicePortInvalid.Name, Namespace: proxyInvalidServicePortInvalid.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidServiceInvalid.Generation).
				WithError("ServiceError", "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: port "9999" on service "roots/home" not matched`),
		},
	})

	proxyValidExampleCom := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidReuseExampleCom := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-example",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "conflicting proxies due to fqdn reuse", testcase{
		objs: []interface{}{proxyValidExampleCom, proxyValidReuseExampleCom},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidExampleCom.Name, Namespace: proxyValidExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidExampleCom.Generation).
				WithError("VirtualHostError", "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`),
			{Name: proxyValidReuseExampleCom.Name, Namespace: proxyValidReuseExampleCom.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidReuseExampleCom.Generation).
				WithError("VirtualHostError", "DuplicateVhost", `fqdn "example.com" is used in multiple HTTPProxies: roots/example-com, roots/other-example`),
		},
	})

	proxyRootIncludesRoot := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Includes: []contour_api_v1.Include{{
				Name:      "blog",
				Namespace: "marketing",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxyRootIncludedByRoot := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
				TLS: &contour_api_v1.TLS{
					SecretName: "blog-containersteve-com",
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	run(t, "root proxy including another root", testcase{
		objs: []interface{}{proxyRootIncludesRoot, proxyRootIncludedByRoot},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyRootIncludesRoot.Name, Namespace: proxyRootIncludesRoot.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootIncludesRoot.Generation).
				WithError("VirtualHostError", "DuplicateVhost", `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`),
			{Name: proxyRootIncludedByRoot.Name, Namespace: proxyRootIncludedByRoot.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootIncludedByRoot.Generation).
				WithError("VirtualHostError", "DuplicateVhost", `fqdn "blog.containersteve.com" is used in multiple HTTPProxies: marketing/blog, roots/root-blog`),
		},
	})

	proxyIncludesRootDifferentFQDN := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "blog.containersteve.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "blog",
				Namespace: "marketing",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
			}},
		},
	}

	proxyRootIncludedByRootDiffFQDN := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.containersteve.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	run(t, "root proxy including another root w/ different hostname", testcase{
		objs: []interface{}{proxyIncludesRootDifferentFQDN, proxyRootIncludedByRootDiffFQDN, fixture.ServiceMarketingGreen},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyIncludesRootDifferentFQDN.Name, Namespace: proxyIncludesRootDifferentFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyIncludesRootDifferentFQDN.Generation).
				WithError("IncludeError", "RootIncludesRoot", "root httpproxy cannot include another root httpproxy"),
			{Name: proxyRootIncludedByRootDiffFQDN.Name, Namespace: proxyRootIncludedByRootDiffFQDN.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootIncludedByRootDiffFQDN.Generation).
				Valid(),
		},
	})

	proxyValidIncludeBlogMarketing := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: fixture.ServiceMarketingGreen.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceMarketingGreen.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxyRootValidIncludesBlogMarketing := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root-blog",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      proxyValidIncludeBlogMarketing.Name,
				Namespace: proxyValidIncludeBlogMarketing.Namespace,
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
		},
	}

	run(t, "proxy includes another", testcase{
		objs: []interface{}{proxyValidIncludeBlogMarketing, proxyRootValidIncludesBlogMarketing, fixture.ServiceRootsKuard, fixture.ServiceMarketingGreen},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidIncludeBlogMarketing.Name, Namespace: proxyValidIncludeBlogMarketing.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidIncludeBlogMarketing.Generation).
				Valid(),
			{Name: proxyRootValidIncludesBlogMarketing.Name, Namespace: proxyRootValidIncludesBlogMarketing.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyRootValidIncludesBlogMarketing.Generation).
				Valid(),
		},
	})

	proxyValidWithMirror := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}, {
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}, {
					Name:   fixture.ServiceRootsKuard.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	run(t, "proxy with mirror", testcase{
		objs: []interface{}{proxyValidWithMirror, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyValidWithMirror.Name, Namespace: proxyValidWithMirror.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidWithMirror.Generation).
				Valid(),
		},
	})

	proxyInvalidTwoMirrors := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}, {
					Name:   fixture.ServiceRootsKuard.Name,
					Port:   8080,
					Mirror: true,
				}, {
					Name:   fixture.ServiceRootsKuard.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	run(t, "proxy with two mirrors", testcase{
		objs: []interface{}{proxyInvalidTwoMirrors, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidTwoMirrors.Name, Namespace: proxyInvalidTwoMirrors.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidTwoMirrors.Generation).
				WithError("ServiceError", "OnlyOneMirror", "only one service per route may be nominated as mirror"),
		},
	})

	proxyInvalidDuplicateMatchConditionHeaders := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "1234",
					},
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate route condition headers", testcase{
		objs: []interface{}{proxyInvalidDuplicateMatchConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidDuplicateMatchConditionHeaders.Name, Namespace: proxyInvalidDuplicateMatchConditionHeaders.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidDuplicateMatchConditionHeaders.Generation).
				WithError("RouteError", "HeaderMatchConditionsNotValid", "cannot specify duplicate header 'exact match' conditions in the same route"),
			// {
			// 	Object: proxyInvalidDuplicateMatchConditionHeaders,
			// 	Status: "invalid", Description: "cannot specify duplicate header 'exact match' conditions in the same route",
			// 	Vhost: "example.com",
			// },
		},
	})

	proxyInvalidDuplicateIncludeCondtionHeaders := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "delegated",
				Namespace: "roots",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "abc",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "x-header",
						Exact: "1234",
					},
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}
	proxyValidDelegatedRoots := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "delegated",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate include condition headers", testcase{
		objs: []interface{}{proxyInvalidDuplicateIncludeCondtionHeaders, proxyValidDelegatedRoots, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidDuplicateIncludeCondtionHeaders.Name,
				Namespace: proxyInvalidDuplicateIncludeCondtionHeaders.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidDuplicateIncludeCondtionHeaders.Generation).Valid(),
			{Name: proxyValidDelegatedRoots.Name,
				Namespace: proxyValidDelegatedRoots.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidDelegatedRoots.Generation).
				WithError("RouteError", "HeaderMatchConditionsNotValid", "cannot specify duplicate header 'exact match' conditions in the same route"),
		},
	})

	proxyInvalidRouteConditionHeaders := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						NotExact: "abc",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						NotExact: "1234",
					},
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate valid route condition headers", testcase{
		objs: []interface{}{proxyInvalidRouteConditionHeaders, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidRouteConditionHeaders.Name, Namespace: proxyInvalidRouteConditionHeaders.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidRouteConditionHeaders.Generation).Valid(),
			// {
			// 	Object: proxyInvalidRouteConditionHeaders,
			// 	Status: "valid", Description: "valid HTTPProxy",
			// 	Vhost: "example.com",
			// },
		},
	})

	proxyInvalidMultiplePrefixes := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy with two prefix conditions on route", testcase{
		objs: []interface{}{proxyInvalidMultiplePrefixes, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidMultiplePrefixes.Name, Namespace: proxyInvalidMultiplePrefixes.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidMultiplePrefixes.Generation).
				WithError("RouteError", "PathMatchConditionsNotValid", "route: more than one prefix is not allowed in a condition block"),
		},
	})

	proxyInvalidTwoPrefixesWithInclude := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{
					{
						Prefix: "/api",
					}, {
						Prefix: "/v1",
					},
				},
			}},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidChildTeamA := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: "teama",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy with two prefix conditions orphans include", testcase{
		objs: []interface{}{proxyInvalidTwoPrefixesWithInclude, proxyValidChildTeamA, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidTwoPrefixesWithInclude.Name, Namespace: proxyInvalidTwoPrefixesWithInclude.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidTwoPrefixesWithInclude.Generation).
				WithError("IncludeError", "PathMatchConditionsNotValid", "include: more than one prefix is not allowed in a condition block"),
			{Name: proxyValidChildTeamA.Name, Namespace: proxyValidChildTeamA.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyValidChildTeamA.Generation).
				Orphaned(),
		},
	})

	proxyInvalidPrefixNoSlash := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{
					{
						Prefix: "api",
					},
				},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy with prefix conditions on route that does not start with slash", testcase{
		objs: []interface{}{proxyInvalidPrefixNoSlash, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidPrefixNoSlash.Name, Namespace: proxyInvalidPrefixNoSlash.Namespace}: fixture.NewValidCondition().
				WithGeneration(proxyInvalidPrefixNoSlash.Generation).
				WithError("RouteError", "PathMatchConditionsNotValid", "route: prefix conditions must start with /, api was supplied"),
		},
	})

	proxyInvalidIncludePrefixNoSlash := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "child",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{
					{
						Prefix: "api",
					},
				},
			}},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "proxy with include prefix that does not start with slash", testcase{
		objs: []interface{}{proxyInvalidIncludePrefixNoSlash, proxyValidChildTeamA, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidIncludePrefixNoSlash.Name, Namespace: proxyInvalidIncludePrefixNoSlash.Namespace}: fixture.NewValidCondition().
				WithError("IncludeError", "PathMatchConditionsNotValid", "include: prefix conditions must start with /, api was supplied"),
			{Name: proxyValidChildTeamA.Name, Namespace: proxyValidChildTeamA.Namespace}: fixture.NewValidCondition().
				Orphaned(),
		},
	})

	proxyInvalidTCPProxyIncludeAndService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Include: &contour_api_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: "roots",
				},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "tcpproxy cannot specify services and include", testcase{
		objs: []interface{}{proxyInvalidTCPProxyIncludeAndService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidTCPProxyIncludeAndService.Name, Namespace: proxyInvalidTCPProxyIncludeAndService.Namespace}: fixture.NewValidCondition().
				WithError("TCPProxyError", "NoServicesAndInclude", "cannot specify services and include in the same httpproxy"),
		},
	})

	proxyTCPNoServiceOrInclusion := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{},
		},
	}

	run(t, "tcpproxy empty", testcase{
		objs: []interface{}{proxyTCPNoServiceOrInclusion, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPNoServiceOrInclusion.Name, Namespace: proxyTCPNoServiceOrInclusion.Namespace}: fixture.NewValidCondition().
				WithError("TCPProxyError", "NothingDefined", "either services or inclusion must be specified"),
		},
	})

	proxyTCPIncludesFoo := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "passthrough.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Include: &contour_api_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	run(t, "tcpproxy w/ missing include", testcase{
		objs: []interface{}{proxyTCPIncludesFoo, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: fixture.NewValidCondition().
				WithError("TCPProxyIncludeError", "IncludeNotFound", "include roots/foo not found"),
		},
	})

	proxyValidTCPRoot := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "tcpproxy includes another root", testcase{
		objs: []interface{}{proxyTCPIncludesFoo, proxyValidTCPRoot, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}: fixture.NewValidCondition().
				WithError("TCPProxyIncludeError", "RootIncludesRoot", "root httpproxy cannot include another root httpproxy"),
			{Name: proxyValidTCPRoot.Name, Namespace: proxyValidTCPRoot.Namespace}: fixture.NewValidCondition().Valid(),
		},
	})

	proxyTCPValidChildFoo := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "tcpproxy includes valid child", testcase{
		objs: []interface{}{proxyTCPIncludesFoo, proxyTCPValidChildFoo, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPIncludesFoo.Name, Namespace: proxyTCPIncludesFoo.Namespace}:     fixture.NewValidCondition().Valid(),
			{Name: proxyTCPValidChildFoo.Name, Namespace: proxyTCPValidChildFoo.Namespace}: fixture.NewValidCondition().Valid(),
		},
	})

	proxyInvalidConflictingIncludeConditions := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidBlogTeamA := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "blogteama",
			Name:      "teama",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceTeamAKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyValidBlogTeamB := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "blogteamb",
			Name:      "teamb",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceTeamBKuard.Name,
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate path conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictingIncludeConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidConflictingIncludeConditions.Name,
				Namespace: proxyInvalidConflictingIncludeConditions.Namespace}: fixture.NewValidCondition().
				WithError("IncludeError", "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{Name: proxyValidBlogTeamA.Name, Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Orphaned(),
			{Name: proxyValidBlogTeamB.Name, Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Orphaned(),
		},
	})

	proxyInvalidConflictHeaderConditions := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{{
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate header conditions on an include", testcase{
		objs: []interface{}{proxyInvalidConflictHeaderConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidConflictHeaderConditions.Name,
				Namespace: proxyInvalidConflictHeaderConditions.Namespace}: fixture.NewValidCondition().
				WithError("IncludeError", "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().Orphaned(),
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().Orphaned(),
		},
	})

	proxyInvalidDuplicateHeaderAndPathConditions := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "blogteama",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}, {
				Name:      "blogteamb",
				Namespace: "teamb",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "x-header",
						Contains: "abc",
					},
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "duplicate header+path conditions on an include", testcase{
		objs: []interface{}{proxyInvalidDuplicateHeaderAndPathConditions, proxyValidBlogTeamA, proxyValidBlogTeamB, fixture.ServiceRootsHome, fixture.ServiceTeamAKuard, fixture.ServiceTeamBKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidDuplicateHeaderAndPathConditions.Name,
				Namespace: proxyInvalidDuplicateHeaderAndPathConditions.Namespace}: fixture.NewValidCondition().
				WithError("IncludeError", "DuplicateMatchConditions", "duplicate conditions defined on an include"),
			{Name: proxyValidBlogTeamA.Name,
				Namespace: proxyValidBlogTeamA.Namespace}: fixture.NewValidCondition().
				Orphaned(),
			{Name: proxyValidBlogTeamB.Name,
				Namespace: proxyValidBlogTeamB.Namespace}: fixture.NewValidCondition().
				Orphaned(),
		},
	})

	proxyInvalidMissingInclude := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "child",
			}},
		},
	}

	run(t, "httpproxy w/ missing include", testcase{
		objs: []interface{}{proxyInvalidMissingInclude, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidMissingInclude.Name, Namespace: proxyInvalidMissingInclude.Namespace}: fixture.NewValidCondition().
				WithError("IncludeError", "IncludeNotFound", "include roots/child not found"),
		},
	})

	proxyTCPInvalidMissingService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-tcp-proxy-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: "not-found",
					Port: 8080,
				}},
			},
		},
	}

	run(t, "httpproxy w/ tcpproxy w/ missing service", testcase{
		objs: []interface{}{proxyTCPInvalidMissingService},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPInvalidMissingService.Name, Namespace: proxyTCPInvalidMissingService.Namespace}: fixture.NewValidCondition().
				WithError("TCPProxyError", "UnresolvedServiceRef", `Spec.TCPProxy unresolved service reference: service "roots/not-found" not found`),
		},
	})

	proxyTCPInvalidPortNotMatched := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-proxy-service-missing-port",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 9999,
				}},
			},
		},
	}

	run(t, "httpproxy w/ tcpproxy w/ service missing port", testcase{
		objs: []interface{}{proxyTCPInvalidPortNotMatched, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPInvalidPortNotMatched.Name, Namespace: proxyTCPInvalidPortNotMatched.Namespace}: fixture.NewValidCondition().
				WithError("TCPProxyError", "UnresolvedServiceRef", `Spec.TCPProxy unresolved service reference: port "9999" on service "roots/kuard" not matched`),
		},
	})

	proxyTCPInvalidMissingTLS := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-tls",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "httpproxy w/ tcpproxy missing tls", testcase{
		objs: []interface{}{proxyTCPInvalidMissingTLS},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPInvalidMissingTLS.Name, Namespace: proxyTCPInvalidMissingTLS.Namespace}: fixture.NewValidCondition().
				WithError("TCPProxyError", "TLSMustBeConfigured", "Spec.TCPProxy requires that either Spec.TLS.Passthrough or Spec.TLS.SecretName be set"),
		},
	})

	proxyInvalidMissingServiceWithTCPProxy := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-route-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{Name: "missing", Port: 9000},
				},
			}},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "httpproxy w/ tcpproxy missing service", testcase{
		objs: []interface{}{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyInvalidMissingServiceWithTCPProxy},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidMissingServiceWithTCPProxy.Name, Namespace: proxyInvalidMissingServiceWithTCPProxy.Namespace}: fixture.NewValidCondition().
				WithError("ServiceError", "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: service "roots/missing" not found`),
		},
	})

	proxyRoutePortNotMatchedWithTCP := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-route-service-port",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{Name: fixture.ServiceRootsKuard.Name, Port: 9999},
				},
			}},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "tcpproxy route unmatched service port", testcase{
		objs: []interface{}{fixture.SecretRootsCert, fixture.ServiceRootsKuard, proxyRoutePortNotMatchedWithTCP},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyRoutePortNotMatchedWithTCP.Name, Namespace: proxyRoutePortNotMatchedWithTCP.Namespace}: fixture.NewValidCondition().
				WithError("ServiceError", "ServiceUnresolvedReference", `Spec.Routes unresolved service reference: port "9999" on service "roots/kuard" not matched`),
		},
	})

	proxyTCPValidIncludeChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Include: &contour_api_v1.TCPProxyInclude{
					Name:      "child",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	proxyTCPValidIncludesChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validtcpproxy",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				IncludesDeprecated: &contour_api_v1.TCPProxyInclude{
					Name:      "child",
					Namespace: fixture.ServiceRootsKuard.Namespace,
				},
			},
		},
	}

	proxyTCPValidChild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: fixture.ServiceRootsKuard.Name,
					Port: 8080,
				}},
			},
		},
	}

	run(t, "valid HTTPProxy.TCPProxy - plural", testcase{
		objs: []interface{}{proxyTCPValidIncludesChild, proxyTCPValidChild, fixture.ServiceRootsKuard, fixture.SecretRootsCert},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPValidIncludesChild.Name,
				Namespace: proxyTCPValidIncludesChild.Namespace}: fixture.NewValidCondition().Valid(),
			{Name: proxyTCPValidChild.Name,
				Namespace: proxyTCPValidChild.Namespace}: fixture.NewValidCondition().Valid(),
		},
	})

	run(t, "valid HTTPProxy.TCPProxy", testcase{
		objs: []interface{}{proxyTCPValidIncludeChild, proxyTCPValidChild, fixture.ServiceRootsKuard, fixture.SecretRootsCert},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyTCPValidIncludeChild.Name,
				Namespace: proxyTCPValidIncludeChild.Namespace}: fixture.NewValidCondition().Valid(),
			{Name: proxyTCPValidChild.Name,
				Namespace: proxyTCPValidChild.Namespace}: fixture.NewValidCondition().Valid(),
		},
	})

	// issue 2309
	proxyInvalidNoServices := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-service",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "missing-service.example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: nil, // missing
			}},
		},
	}

	run(t, "invalid HTTPProxy due to empty route.service", testcase{
		objs: []interface{}{proxyInvalidNoServices, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyInvalidNoServices.Name, Namespace: proxyInvalidNoServices.Namespace}: fixture.NewValidCondition().
				WithError("RouteError", "NoServicesPresent", "route.services must have at least one entry"),
		},
	})

	fallbackCertificate := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "invalid fallback certificate passed to contour", testcase{
		fallbackCertificate: &types.NamespacedName{
			Name:      "invalid",
			Namespace: "invalid",
		},
		objs: []interface{}{fallbackCertificate, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace}: fixture.NewValidCondition().
				WithError("TLSError", "FallbackNotValid", `Spec.Virtualhost.TLS Secret "invalid/invalid" fallback certificate is invalid: Secret not found`),
		},
	})

	run(t, "fallback certificate requested but cert not configured in contour", testcase{
		objs: []interface{}{fallbackCertificate, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: fallbackCertificate.Name,
				Namespace: fallbackCertificate.Namespace}: fixture.NewValidCondition().
				WithError("TLSError", "FallbackNotPresent", "Spec.Virtualhost.TLS enabled fallback but the fallback Certificate Secret is not configured in Contour configuration file"),
		},
	})

	fallbackCertificateWithClientValidation := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "roots",
			Name:      "example",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						CACertificate: "something",
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "home",
					Port: 8080,
				}},
			}},
		},
	}

	run(t, "fallback certificate requested and clientValidation also configured", testcase{
		objs: []interface{}{fallbackCertificateWithClientValidation, fixture.SecretRootsFallback, fixture.SecretRootsCert, fixture.ServiceRootsHome},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: fallbackCertificateWithClientValidation.Name,
				Namespace: fallbackCertificateWithClientValidation.Namespace}: fixture.NewValidCondition().
				WithError("TLSError", "TLSIncompatibleFeatures", "Spec.Virtualhost.TLS fallback & client validation are incompatible"),
		},
	})

	tlsPassthroughAndValidation := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						CACertificate: "aCAcert",
					},
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{},
		},
	}

	run(t, "passthrough and client auth are incompatible tlsPassthroughAndValidation", testcase{
		objs: []interface{}{fixture.SecretRootsCert, tlsPassthroughAndValidation},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: tlsPassthroughAndValidation.Name, Namespace: tlsPassthroughAndValidation.Namespace}: fixture.NewValidCondition().
				WithError("TLSError", "TLSIncompatibleFeatures", "Spec.VirtualHost.TLS passthrough cannot be combined with tls.clientValidation"),
		},
	})

	tlsPassthroughAndSecretName := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
					SecretName:  fixture.SecretRootsCert.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{},
		},
	}

	run(t, "tcpproxy with TLS passthrough and secret name both specified", testcase{
		objs: []interface{}{
			fixture.SecretRootsCert,
			tlsPassthroughAndSecretName,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: "invalid", Namespace: fixture.ServiceRootsKuard.Namespace}: fixture.NewValidCondition().
				WithError("TLSError", "TLSConfigNotValid", "Spec.VirtualHost.TLS: both Passthrough and SecretName were specified"),
		},
	})

	tlsNoPassthroughOrSecretName := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "tcpproxy.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: false,
					SecretName:  "",
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{},
		},
	}

	run(t, "httpproxy w/ tcpproxy with neither TLS passthrough nor secret name specified", testcase{
		objs: []interface{}{
			fixture.SecretRootsCert,
			tlsNoPassthroughOrSecretName,
		},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: "invalid", Namespace: fixture.ServiceRootsKuard.Namespace}: fixture.NewValidCondition().
				WithError("TLSError", "TLSConfigNotValid", "Spec.VirtualHost.TLS: neither Passthrough nor SecretName were specified"),
		},
	})

	emptyProxy := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty",
			Namespace: "roots",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
		},
	}

	run(t, "proxy with no routes, includes, or tcpproxy is invalid", testcase{
		objs: []interface{}{emptyProxy},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: emptyProxy.Name, Namespace: emptyProxy.Namespace}: fixture.NewValidCondition().
				WithError("SpecError", "NothingDefined", "HTTPProxy.Spec must have at least one Route, Include, or a TCPProxy"),
		},
	})

	invalidRequestHeadersPolicyService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidRHPService",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
						RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
							Set: []contour_api_v1.HeaderValue{{
								Name:  "Host",
								Value: "external.com",
							}},
						},
					},
				},
			}},
		},
	}

	run(t, "requestHeadersPolicy, Host header invalid on Service", testcase{
		objs: []interface{}{invalidRequestHeadersPolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: invalidRequestHeadersPolicyService.Name, Namespace: invalidRequestHeadersPolicyService.Namespace}: fixture.NewValidCondition().
				WithError("ServiceError", "RequestHeadersPolicyInvalid", `rewriting "Host" header is not supported on request headers`),
		},
	})

	invalidResponseHeadersPolicyService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidRHPService",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
						ResponseHeadersPolicy: &contour_api_v1.HeadersPolicy{
							Set: []contour_api_v1.HeaderValue{{
								Name:  "Host",
								Value: "external.com",
							}},
						},
					},
				},
			}},
		},
	}

	run(t, "responseHeadersPolicy, Host header invalid on Service", testcase{
		objs: []interface{}{invalidResponseHeadersPolicyService, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: invalidResponseHeadersPolicyService.Name, Namespace: invalidResponseHeadersPolicyService.Namespace}: fixture.NewValidCondition().
				WithError("ServiceError", "ResponseHeadersPolicyInvalid", `rewriting "Host" header is not supported on response headers`),
		},
	})

	invalidResponseHeadersPolicyRoute := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidRHPRoute",
			Namespace: fixture.ServiceRootsKuard.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{
					{
						Name: fixture.ServiceRootsKuard.Name,
						Port: 8080,
					},
				},
				ResponseHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "external.com",
					}},
				},
			}},
		},
	}

	run(t, "responseHeadersPolicy, Host header invalid on Route", testcase{
		objs: []interface{}{invalidResponseHeadersPolicyRoute, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: invalidResponseHeadersPolicyRoute.Name, Namespace: invalidResponseHeadersPolicyRoute.Namespace}: fixture.NewValidCondition().
				WithError("RouteError", "ResponseHeaderPolicyInvalid", `rewriting "Host" header is not supported on response headers`),
		},
	})

	proxyAuthFallback := fixture.NewProxy("roots/fallback-incompat").
		WithSpec(contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "invalid.com",
				TLS: &contour_api_v1.TLS{
					SecretName:                "ssl-cert",
					EnableFallbackCertificate: true,
				},
				Authorization: &contour_api_v1.AuthorizationServer{
					ExtensionServiceRef: contour_api_v1.ExtensionServiceReference{
						Namespace: "auth",
						Name:      "extension",
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{Name: "app-server", Port: 80}},
			}},
		})

	run(t, "fallback and client auth is invalid", testcase{
		objs: []interface{}{fixture.SecretRootsCert, proxyAuthFallback},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{Name: proxyAuthFallback.Name, Namespace: proxyAuthFallback.Namespace}: fixture.NewValidCondition().
				WithError("TLSError", "TLSIncompatibleFeatures", "Spec.Virtualhost.TLS fallback & client authorization are incompatible"),
		},
	})

	invalidResponseTimeout := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-timeouts",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Services: []contour_api_v1.Service{
						{
							Name: fixture.ServiceRootsKuard.Name,
						},
					},
					TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
						Response: "invalid-val",
					},
				},
			},
		},
	}

	run(t, "proxy with invalid response timeout value is invalid", testcase{
		objs: []interface{}{invalidResponseTimeout, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{
				Name:      invalidResponseTimeout.Name,
				Namespace: invalidResponseTimeout.Namespace,
			}: fixture.NewValidCondition().WithError("RouteError", "TimeoutPolicyNotValid", `route.timeoutPolicy failed to parse: error parsing response timeout: unable to parse timeout string "invalid-val": time: invalid duration "invalid-val"`),
		},
	})

	invalidIdleTimeout := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: fixture.ServiceRootsKuard.Namespace,
			Name:      "invalid-timeouts",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Services: []contour_api_v1.Service{
						{
							Name: fixture.ServiceRootsKuard.Name,
						},
					},
					TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
						Idle: "invalid-val",
					},
				},
			},
		},
	}

	run(t, "proxy with invalid idle timeout value is invalid", testcase{
		objs: []interface{}{invalidIdleTimeout, fixture.ServiceRootsKuard},
		want: map[types.NamespacedName]contour_api_v1.DetailedCondition{
			{
				Name:      invalidIdleTimeout.Name,
				Namespace: invalidIdleTimeout.Namespace,
			}: fixture.NewValidCondition().WithError("RouteError", "TimeoutPolicyNotValid", `route.timeoutPolicy failed to parse: error parsing idle timeout: unable to parse timeout string "invalid-val": time: invalid duration "invalid-val"`),
		},
	})

}
