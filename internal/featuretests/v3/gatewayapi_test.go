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

	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var gatewayTLS = &gatewayapi_v1alpha1.Gateway{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "contour",
		Namespace: "projectcontour",
	},
	Spec: gatewayapi_v1alpha1.GatewaySpec{
		Listeners: []gatewayapi_v1alpha1.Listener{{
			Port:     80,
			Protocol: "HTTP",
			Routes: gatewayapi_v1alpha1.RouteBindingSelector{
				Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
					From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
				},
				Kind: dag.KindHTTPRoute,
			},
		}, {
			Port:     443,
			Protocol: "HTTPS",
			TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
				CertificateRef: &gatewayapi_v1alpha1.LocalObjectReference{
					Group: "core",
					Kind:  "Secret",
					Name:  "tlscert",
				},
			},
			Routes: gatewayapi_v1alpha1.RouteBindingSelector{
				Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
					From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
				},
				Kind: dag.KindHTTPRoute,
			},
		}},
	},
}

var gatewayHTTP = &gatewayapi_v1alpha1.Gateway{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "contour",
		Namespace: "projectcontour",
	},
	Spec: gatewayapi_v1alpha1.GatewaySpec{
		Listeners: []gatewayapi_v1alpha1.Listener{{
			Port:     80,
			Protocol: "HTTP",
			Routes: gatewayapi_v1alpha1.RouteBindingSelector{
				Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
					From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
				},
				Kind: dag.KindHTTPRoute,
			},
		}},
	},
}

func TestGateway_TLS(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tlscert",
			Namespace: "projectcontour",
		},
		Type: v1.SecretTypeTLS,
		Data: featuretests.Secretdata(featuretests.CERTIFICATE, featuretests.RSA_PRIVATE_KEY),
	}

	rh.OnAdd(sec1)

	rh.OnAdd(gatewayTLS)

	rh.OnAdd(&gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/blog"),
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc2"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(1),
				}},
			}, {
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/"),
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc1"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(10),
				}},
			}},
		},
	})

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("https/test.projectcontour.io",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/blog"),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/blog"),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	c.Request(listenerType, "ingress_https").Equals(&envoy_discovery_v3.DiscoveryResponse{
		TypeUrl: listenerType,
		Resources: resources(t,
			&envoy_listener_v3.Listener{
				Name:    "ingress_https",
				Address: envoy_v3.SocketAddress("0.0.0.0", 8443),
				ListenerFilters: envoy_v3.ListenerFilters(
					envoy_v3.TLSInspector(),
				),
				FilterChains: appendFilterChains(
					filterchaintls("test.projectcontour.io", sec1,
						httpsFilterFor("test.projectcontour.io"),
						nil, "h2", "http/1.1"),
				),
				SocketOptions: envoy_v3.TCPKeepaliveSocketOptions(),
			},
		),
	})
}

func TestGateway_RouteConflict(t *testing.T) {
	rh, c, done := setup(t)
	defer done()

	rh.OnAdd(fixture.NewService("svc1").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewService("svc2").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(fixture.NewService("svc3").
		WithPorts(v1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}),
	)

	rh.OnAdd(gatewayHTTP)

	httpRouteBasic := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
			CreationTimestamp: metav1.Now(),
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/blog"),
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc2"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(1),
				}},
			}, {
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/"),
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc1"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(10),
				}},
			}},
		},
	}

	rh.OnAdd(httpRouteBasic)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/blog"),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// This HTTPRoute conflicts with "basic" since it has the same
	// prefix path of `/blog` and is dropped since the "basic" route
	// is older than this resource.
	httpRouteBasic2 := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic2",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
			CreationTimestamp: metav1.Now(),
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/blog"),
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc3"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(1),
				}},
			}},
		},
	}

	rh.OnAdd(httpRouteBasic2)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/blog"),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// Deleting the "basic" HTTPRoute should now allow the "basic2"
	// resource to become valid.
	rh.OnDelete(httpRouteBasic)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/blog"),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// Add back the "basic" HTTPRoute, but now the "basic2" resource
	// is older, so it should take precedence.
	httpRouteBasic.CreationTimestamp = metav1.Now()
	rh.OnAdd(httpRouteBasic)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match:  routePrefix("/blog"),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	// Cleanup
	rh.OnDelete(httpRouteBasic)
	rh.OnDelete(httpRouteBasic2)

	httpRouteHeaders := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headers",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
			CreationTimestamp: metav1.Now(),
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/blog"),
					},
					Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
						Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
						Values: map[string]string{"foo": "bar"},
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc2"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(1),
				}},
			}, {
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/"),
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc1"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(10),
				}},
			}},
		},
	}

	rh.OnAdd(httpRouteHeaders)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match: routePrefix("/blog", dag.HeaderMatchCondition{
							Name:      "foo",
							Value:     "bar",
							MatchType: "exact",
							Invert:    false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	httpRouteHeaders2 := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headers2",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
			CreationTimestamp: metav1.Now(),
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/blog"),
					},
					Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
						Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
						Values: map[string]string{"foo": "bar"},
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc2"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(1),
				}},
			}},
		},
	}

	// Adding "headers2" should be rejected since the path /blog &
	// headers foo:bar match "headers" resource, but "headers" has an
	// older timestamp.
	rh.OnAdd(httpRouteHeaders2)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match: routePrefix("/blog", dag.HeaderMatchCondition{
							Name:      "foo",
							Value:     "bar",
							MatchType: "exact",
							Invert:    false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					}, &envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})

	httpRouteHeaders2Fixed := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headers2",
			Namespace: "default",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
			CreationTimestamp: metav1.Now(),
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
					Path: &gatewayapi_v1alpha1.HTTPPathMatch{
						Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
						Value: pointer.StringPtr("/blog"),
					},
					Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
						Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
						Values: map[string]string{"something": "else"},
					},
				}},
				ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
					ServiceName: pointer.StringPtr("svc3"),
					Port:        gatewayPort(80),
					Weight:      pointer.Int32Ptr(1),
				}},
			}},
		},
	}

	// Make the headers on "headers2" unique from "headers"
	// now making it valid and should be processed.
	rh.OnUpdate(httpRouteHeaders2, httpRouteHeaders2Fixed)

	c.Request(routeType).Equals(&envoy_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			envoy_v3.RouteConfiguration("ingress_http",
				envoy_v3.VirtualHost("test.projectcontour.io",
					&envoy_route_v3.Route{
						Match: routePrefix("/blog", dag.HeaderMatchCondition{
							Name:      "foo",
							Value:     "bar",
							MatchType: "exact",
							Invert:    false,
						}),
						Action: routeCluster("default/svc2/80/da39a3ee5e"),
					},
					&envoy_route_v3.Route{
						Match: routePrefix("/blog", dag.HeaderMatchCondition{
							Name:      "something",
							Value:     "else",
							MatchType: "exact",
							Invert:    false,
						}),
						Action: routeCluster("default/svc3/80/da39a3ee5e"),
					}, &envoy_route_v3.Route{
						Match:  routePrefix("/"),
						Action: routeCluster("default/svc1/80/da39a3ee5e"),
					},
				),
			),
		),
		TypeUrl: routeType,
	})
}

func gatewayPort(port int) *gatewayapi_v1alpha1.PortNumber {
	p := gatewayapi_v1alpha1.PortNumber(port)
	return &p
}

func pathMatchTypePtr(pmt gatewayapi_v1alpha1.PathMatchType) *gatewayapi_v1alpha1.PathMatchType {
	return &pmt
}

func headerMatchTypePtr(hmt gatewayapi_v1alpha1.HeaderMatchType) *gatewayapi_v1alpha1.HeaderMatchType {
	return &hmt
}

func routeSelectTypePtr(rst gatewayapi_v1alpha1.RouteSelectType) *gatewayapi_v1alpha1.RouteSelectType {
	return &rst
}

func gatewayAllowTypePtr(gwType gatewayapi_v1alpha1.GatewayAllowType) *gatewayapi_v1alpha1.GatewayAllowType {
	return &gwType
}
