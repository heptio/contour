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
	"errors"
	"net/http"
	"testing"
	"time"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func gatewayPort(port int) *gatewayapi_v1alpha1.PortNumber {
	p := gatewayapi_v1alpha1.PortNumber(port)
	return &p
}

func TestDAGInsertGatewayAPI(t *testing.T) {

	kuardService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "projectcontour",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	kuardService2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard2",
			Namespace: "projectcontour",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	kuardService3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard3",
			Namespace: "projectcontour",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	kuardServiceCustomNs := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "custom",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	blogService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blogsvc",
			Namespace: "projectcontour",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	gatewayWithSelector := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSame),
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "contour",
						},
						MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      "type",
							Operator: "In",
							Values:   []string{"controller"},
						}},
					},
				},
			}},
		},
	}

	gatewayWithSameNamespace := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSame),
					},
				},
			}},
		},
	}

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tlscert",
			Namespace: "projectcontour",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	gatewayWithOnlyTLS := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     443,
				Protocol: gatewayapi_v1alpha1.HTTPSProtocolType,
				TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
					CertificateRef: &gatewayapi_v1alpha1.LocalObjectReference{
						Group: "core",
						Kind:  "Secret",
						Name:  sec1.Name,
					},
				},
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
					},
				},
			}},
		},
	}

	gatewayWithTLSandHTTP := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
					},
				},
			}, {
				Port:     443,
				Protocol: gatewayapi_v1alpha1.HTTPSProtocolType,
				TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
					CertificateRef: &gatewayapi_v1alpha1.LocalObjectReference{
						Group: "core",
						Kind:  "Secret",
						Name:  sec1.Name,
					},
				},
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
					},
				},
			}},
		},
	}

	gatewaywithtlsDifferentselectors := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
					},
					Selector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      "protocol",
							Operator: "In",
							Values:   []string{"http"},
						}},
					},
				},
			}, {
				Port:     443,
				Protocol: gatewayapi_v1alpha1.HTTPSProtocolType,
				TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
					CertificateRef: &gatewayapi_v1alpha1.LocalObjectReference{
						Group: "core",
						Kind:  "Secret",
						Name:  sec1.Name,
					},
				},
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
					},
					Selector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      "protocol",
							Operator: "In",
							Values:   []string{"https"},
						}},
					},
				},
			}},
		},
	}

	genericHTTPRoute := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic",
			Namespace: "projectcontour",
			Labels: map[string]string{
				"app":      "contour",
				"type":     "controller",
				"protocol": "http",
			},
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
				ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
			}},
		},
	}

	httpRouteProtocolHTTPS := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basictls",
			Namespace: "projectcontour",
			Labels: map[string]string{
				"app":      "contour",
				"type":     "controller",
				"protocol": "https",
			},
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Gateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
			},
			Hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
				Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
				ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
			}},
		},
	}

	gatewayWithAllNamespace := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
					},
				},
			}},
		},
	}

	gatewayWithNamespaceSelector := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "contour",
							},
							MatchExpressions: []metav1.LabelSelectorRequirement{{
								Key:      "type",
								Operator: "In",
								Values:   []string{"controller"},
							}},
						},
					},
				},
			}},
		},
	}

	gatewayWithNamespaceSelectorNotMatching := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
						From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "contour",
							},
							MatchExpressions: []metav1.LabelSelectorRequirement{{
								Key:      "type",
								Operator: "In",
								Values:   []string{"controller"},
							}},
						},
					},
				},
			}},
		},
	}

	gatewayNoSelector := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
				},
			}},
		},
	}

	gatewaySelectorNotMatching := &gatewayapi_v1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contour",
			Namespace: "projectcontour",
		},
		Spec: gatewayapi_v1alpha1.GatewaySpec{
			Listeners: []gatewayapi_v1alpha1.Listener{{
				Port:     80,
				Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
				Routes: gatewayapi_v1alpha1.RouteBindingSelector{
					Kind: KindHTTPRoute,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"not": "matching",
						},
						MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      "something",
							Operator: "In",
							Values:   []string{"else"},
						}},
					},
				},
			}},
		},
	}

	tests := map[string]struct {
		objs                         []interface{}
		disablePermitInsecure        bool
		fallbackCertificateName      string
		fallbackCertificateNamespace string
		gateway                      *gatewayapi_v1alpha1.Gateway
		want                         []Vertex
	}{
		"insert basic single route, single hostname": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				genericHTTPRoute,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		// Test that a gateway without a Selector will select objects.
		"insert basic single route, single hostname, gateway no selector": {
			gateway: gatewayNoSelector,
			objs: []interface{}{
				kuardService,
				genericHTTPRoute,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		// Test that a gateway without a Selector will select objects.
		"insert basic single route, single hostname, gateway same namespace selector": {
			gateway: gatewayWithSameNamespace,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"insert basic single route, single hostname, gateway same namespace selector, route different namespace": {
			gateway: gatewayWithSameNamespace,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway All namespace selector": {
			gateway: gatewayWithAllNamespace,
			objs: []interface{}{
				kuardServiceCustomNs,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardServiceCustomNs))),
					),
				},
			),
		},
		"insert basic single route, single hostname, gateway From namespace selector": {
			gateway: gatewayWithNamespaceSelector,
			objs: []interface{}{
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "custom",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
				},
				kuardServiceCustomNs,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardServiceCustomNs))),
					),
				},
			),
		},
		// HTTPRoute is in a different namespace than the Gateway,
		// but will be allowed since "All" is set.
		"RouteGateways with GatewayAllowType: All": {
			gateway: gatewayNoSelector,
			objs: []interface{}{
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "custom",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
				},
				kuardServiceCustomNs,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardServiceCustomNs))),
					),
				},
			),
		},
		// HTTPRoute is in a different namespace than the Gateway,
		// and is rejected since "SameNamespace" is set.
		"HTTPRoute doesn't match with RouteGateways.GatewayAllowType: SameNamespace": {
			gateway: gatewayNoSelector,
			objs: []interface{}{
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "custom",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
				},
				kuardServiceCustomNs,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		// HTTPRoute is in the same namespace of the Gateway,
		// and is allowed since "SameNamespace" is set.
		"HTTPRoute matches with RouteGateways.GatewayAllowType: SameNamespace": {
			gateway: gatewayNoSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		// HTTPRoute references same Gateway is configured with
		// in the FromList.
		"HTTPRoute matches with RouteGateways.GatewayAllowType: FromList": {
			gateway: gatewayNoSelector,
			objs: []interface{}{
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "custom",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
				},
				kuardServiceCustomNs,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowFromList),
							GatewayRefs: []gatewayapi_v1alpha1.GatewayReference{{
								Name:      "contour",
								Namespace: "projectcontour",
							}},
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardServiceCustomNs))),
					),
				},
			),
		},
		// HTTPRoute references different Gateway is configured with
		// in the FromList.
		"HTTPRoute doesn't match with RouteGateways.GatewayAllowType: FromList": {
			gateway: gatewayNoSelector,
			objs: []interface{}{
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "custom",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
				},
				kuardServiceCustomNs,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowFromList),
							GatewayRefs: []gatewayapi_v1alpha1.GatewayReference{{
								Name:      "wrong",
								Namespace: "reference",
							}},
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		// Issue: https://github.com/projectcontour/contour/issues/3591
		"one gateway with two httproutes, different hostnames": {
			gateway: gatewayWithNamespaceSelector,
			objs: []interface{}{
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "custom",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
				},
				kuardServiceCustomNs,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic-two",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"another.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("another.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardServiceCustomNs))),
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardServiceCustomNs))),
					),
				},
			),
		},
		"insert basic single route, single hostname, gateway From namespace selector, not matching": {
			gateway: gatewayWithNamespaceSelectorNotMatching,
			objs: []interface{}{
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "custom",
						Labels: map[string]string{
							"app":  "notmatch",
							"type": "someother",
						},
					},
				},
				kuardServiceCustomNs,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		// Test that a gateway selector doesn't select routes that do not match.
		"insert basic single route, single hostname which doesn't match gateway's selector": {
			gateway: gatewaySelectorNotMatching,
			objs: []interface{}{
				kuardServiceCustomNs,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "custom",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		// Test that a gateway selector kind that doesn't match.
		"insert gateway with selector kind that doesn't match": {
			gateway: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: "INVALID",
						},
					}},
				},
			},
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		// Test that a gateway selector group that doesn't match.
		"insert gateway with selector group that doesn't match": {
			gateway: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind:  "HTTPRoute",
							Group: pointer.StringPtr("INVALID"),
						},
					}},
				},
			},
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		"insert basic multiple routes, single hostname": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				blogService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}, {
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
							ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							prefixrouteHTTPRoute("/", service(kuardService)), prefixrouteHTTPRoute("/blog", service(blogService))),
					),
				},
			),
		},
		"multiple hosts": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
							"test2.projectcontour.io",
							"test3.projectcontour.io",
							"test4.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
						virtualhost("test2.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
						virtualhost("test3.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
						virtualhost("test4.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"no host defined": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"wildcard hostname": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"*.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"invalid hostnames - IP": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"192.168.122.1",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		"invalid hostnames - with port": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io:80",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		"invalid hostnames - wildcard label by itself": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"*",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		// If the ServiceName referenced from an HTTPRoute is missing,
		// the route should return an HTTP503.
		"missing service": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", directResponseRoute("/", http.StatusServiceUnavailable)),
					),
				},
			),
		},
		// If port is not defined the route will be marked as invalid (#3352).
		"missing port": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "default",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches: httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
								ServiceName: pointer.StringPtr("kuard"),
								Port:        nil,
							}},
						}},
					},
				},
			},
			want: listeners(),
		},
		"insert basic single route with exact path match": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchExact, "/blog"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							exactrouteHTTPRoute("/blog", service(kuardService))),
					),
				},
			),
		},
		// Single host with single route containing multiple prefixes to the same service.
		"insert basic single route with multiple prefixes, single hostname": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
								Path: &gatewayapi_v1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
									Value: pointer.StringPtr("/"),
								},
							}, {
								Path: &gatewayapi_v1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
									Value: pointer.StringPtr("/blog"),
								},
							}, {
								Path: &gatewayapi_v1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
									Value: pointer.StringPtr("/tech"),
								},
							}},
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							prefixrouteHTTPRoute("/", service(kuardService)),
							prefixrouteHTTPRoute("/blog", service(kuardService)),
							prefixrouteHTTPRoute("/tech", service(kuardService))),
					),
				},
			),
		},
		"insert basic single route, single hostname, gateway with TLS, HTTP protocol is ignored": {
			gateway: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     443,
						Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
						TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
							CertificateRef: &gatewayapi_v1alpha1.LocalObjectReference{
								Group: "core",
								Kind:  "Secret",
								Name:  sec1.Name,
							},
						},
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: KindHTTPRoute,
							Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
							},
						},
					}},
				},
			},
			objs: []interface{}{
				sec1,
				kuardService,
				genericHTTPRoute,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							prefixrouteHTTPRoute("/", service(kuardService)),
						)),
				},
			),
		},
		"insert basic single route, single hostname, gateway with TLS, HTTPS protocol missing certificateRef": {
			gateway: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     443,
						Protocol: gatewayapi_v1alpha1.HTTPSProtocolType,
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: KindHTTPRoute,
							Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
							},
						},
					}},
				},
			},
			objs: []interface{}{
				sec1,
				kuardService,
				genericHTTPRoute,
			},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway with TLS": {
			gateway: gatewayWithOnlyTLS,
			objs: []interface{}{
				sec1,
				kuardService,
				genericHTTPRoute,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "test.projectcontour.io",
								ListenerName: "ingress_https",
								routes:       routes(prefixrouteHTTPRoute("/", service(kuardService))),
							},
							Secret: secret(sec1),
						},
					),
				},
			),
		},
		"insert basic single route, single hostname, gateway with missing TLS certificate": {
			gateway: gatewayWithOnlyTLS,
			objs: []interface{}{
				kuardService,
				genericHTTPRoute,
			},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway with invalid TLS certificate": {
			gateway: gatewayWithOnlyTLS,
			objs: []interface{}{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tlscert",
						Namespace: "projectcontour",
					},
					Type: v1.SecretTypeTLS,
					Data: secretdata("wrong", "wronger"),
				},
				kuardService,
				genericHTTPRoute,
			},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway with TLS & Insecure Listeners": {
			gateway: gatewayWithTLSandHTTP,
			objs: []interface{}{
				sec1,
				blogService,
				httpRouteProtocolHTTPS,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "test.projectcontour.io",
								ListenerName: "ingress_https",
								routes:       routes(prefixrouteHTTPRoute("/", service(blogService))),
							},
							Secret: secret(sec1),
						},
					),
				},
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(blogService))),
					),
				},
			),
		},
		"TLS Listener Gateway CertificateRef must be type core.Secret": {
			gateway: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     443,
						Protocol: gatewayapi_v1alpha1.HTTPSProtocolType,
						TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
							CertificateRef: &gatewayapi_v1alpha1.LocalObjectReference{
								Group: "custom",
								Kind:  "shhhh",
								Name:  sec1.Name,
							},
						},
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: KindHTTPRoute,
							Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
							},
						},
					}},
				},
			},
			objs: []interface{}{
				sec1,
				blogService,
				httpRouteProtocolHTTPS,
			},
			want: listeners(),
		},
		"TLS Listener Gateway CertificateRef must be specified": {
			gateway: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     443,
						Protocol: gatewayapi_v1alpha1.HTTPSProtocolType,
						TLS:      &gatewayapi_v1alpha1.GatewayTLSConfig{},
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: KindHTTPRoute,
							Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
							},
						},
					}},
				},
			},
			objs: []interface{}{
				sec1,
				blogService,
				httpRouteProtocolHTTPS,
			},
			want: listeners(),
		},
		"No valid hostnames defined": {
			gateway: gatewayWithAllNamespace,
			objs: []interface{}{
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":      "contour",
							"type":     "controller",
							"protocol": "https",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"*.*.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
						}},
					},
				},
			},
			want: listeners(),
		},
		"Invalid listener protocol type (TCP)": {
			gateway: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1alpha1.TCPProtocolType,
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: KindHTTPRoute,
							Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
							},
						},
					}},
				},
			},
			objs: []interface{}{genericHTTPRoute},
			want: listeners(),
		},
		"Invalid listener protocol type (UDP)": {
			gateway: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1alpha1.UDPProtocolType,
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: KindHTTPRoute,
							Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
							},
						},
					}},
				},
			},
			objs: []interface{}{genericHTTPRoute},
			want: listeners(),
		},
		"Invalid listener protocol type (custom)": {
			gateway: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     80,
						Protocol: "projectcontour.io/HTTPUDP",
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: KindHTTPRoute,
							Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
							},
						},
					}},
				},
			},
			objs: []interface{}{genericHTTPRoute},
			want: listeners(),
		},
		"insert basic single route, single hostname, gateway with TLS & Insecure Listeners, different selectors": {
			gateway: gatewaywithtlsDifferentselectors,
			objs: []interface{}{
				sec1,
				kuardService,
				blogService,
				genericHTTPRoute,
				httpRouteProtocolHTTPS,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "test.projectcontour.io",
								ListenerName: "ingress_https",
								routes:       routes(prefixrouteHTTPRoute("/", service(blogService))),
							},
							Secret: secret(sec1),
						},
					),
				},
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io", prefixrouteHTTPRoute("/", service(kuardService))),
					),
				},
			),
		},
		"insert basic single route with single header match and path match": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
								Path: &gatewayapi_v1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
									Value: pointer.StringPtr("/"),
								},
								Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
									Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
									Values: map[string]string{"foo": "bar"},
								},
							}},
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "foo", Value: "bar", MatchType: "exact", Invert: false},
							},
							Clusters: clustersWeight(service(kuardService)),
						}),
					),
				},
			),
		},
		"insert two routes with single header match, path match and header match": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{
							{
								Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{
									{
										Path: &gatewayapi_v1alpha1.HTTPPathMatch{
											Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
											Value: pointer.StringPtr("/blog"),
										},
									}, {
										Path: &gatewayapi_v1alpha1.HTTPPathMatch{
											Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
											Value: pointer.StringPtr("/tech"),
										},
										Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
											Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
											Values: map[string]string{"foo": "bar"},
										},
									},
								},
								ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
							}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/blog"),
							Clusters:           clustersWeight(service(kuardService)),
						},
						&Route{
							PathMatchCondition: prefixString("/tech"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "foo", Value: "bar", MatchType: "exact", Invert: false},
							},
							Clusters: clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},
		"insert two routes with single header match without explicit path match": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
								Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
									Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
									Values: map[string]string{"foo": "bar"},
								},
							}},
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "foo", Value: "bar", MatchType: "exact", Invert: false},
							},
							Clusters: clustersWeight(service(kuardService)),
						},
					)),
				},
			),
		},
		"Route rule with request header modifier": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
							Filters: []gatewayapi_v1alpha1.HTTPRouteFilter{{
								Type: gatewayapi_v1alpha1.HTTPRouteFilterRequestHeaderModifier,
								RequestHeaderModifier: &gatewayapi_v1alpha1.HTTPRequestHeaderFilter{
									Set: map[string]string{"custom-header-set": "foo-bar", "Host": "bar.com"},
									Add: map[string]string{"custom-header-add": "foo-bar"},
								},
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustersWeight(service(kuardService)),
							RequestHeadersPolicy: &HeadersPolicy{
								Set: map[string]string{
									"Custom-Header-Set": "foo-bar", // Verify the header key is canonicalized.
								},
								Add: map[string]string{
									"Custom-Header-Add": "foo-bar", // Verify the header key is canonicalized.
								},
								HostRewrite: "bar.com",
							},
						},
					)),
				},
			),
		},
		"HTTP forward with request header modifier": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches: httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
								ServiceName: pointer.StringPtr("kuard"),
								Port:        gatewayPort(8080),
								Filters: []gatewayapi_v1alpha1.HTTPRouteFilter{{
									Type: gatewayapi_v1alpha1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1alpha1.HTTPRequestHeaderFilter{
										Set: map[string]string{"custom-header-set": "foo-bar", "Host": "bar.com"},
										Add: map[string]string{"custom-header-add": "foo-bar"},
									},
								}},
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clusterHeaders(map[string]string{"Custom-Header-Set": "foo-bar"}, map[string]string{"Custom-Header-Add": "foo-bar"}, nil, "bar.com", service(kuardService)),
						},
					)),
				},
			),
		},
		"Route rule with invalid request header modifier": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{
							{
								Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
								ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
								Filters: []gatewayapi_v1alpha1.HTTPRouteFilter{{
									Type: gatewayapi_v1alpha1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapi_v1alpha1.HTTPRequestHeaderFilter{
										Set: map[string]string{"custom-header-set": "foo-bar", "Host": "bar.com"},
										Add: map[string]string{"!invalid-header-add": "foo-bar"},
									},
								}},
							}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustersWeight(service(kuardService)),
							RequestHeadersPolicy: &HeadersPolicy{
								Set:         map[string]string{"Custom-Header-Set": "foo-bar"},
								Add:         map[string]string{}, // Invalid header should not be set.
								HostRewrite: "bar.com",
							},
						},
					)),
				},
			),
		},
		"HTTP forward with invalid request header modifier": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{
							{
								Matches: httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
								ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
									ServiceName: pointer.StringPtr("kuard"),
									Port:        gatewayPort(8080),
									Filters: []gatewayapi_v1alpha1.HTTPRouteFilter{{
										Type: gatewayapi_v1alpha1.HTTPRouteFilterRequestHeaderModifier,
										RequestHeaderModifier: &gatewayapi_v1alpha1.HTTPRequestHeaderFilter{
											Set: map[string]string{"custom-header-set": "foo-bar", "Host": "bar.com"},
											Add: map[string]string{"!invalid-header-add": "foo-bar"},
										},
									}},
								}},
							}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clusterHeaders(map[string]string{"Custom-Header-Set": "foo-bar"}, map[string]string{}, nil, "bar.com", service(kuardService)),
						},
					)),
				},
			),
		},
		"different weights for multiple forwardTos": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				kuardService2,
				kuardService3,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches: httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwards(
								httpRouteForwardTo("kuard", 8080, 5),
								httpRouteForwardTo("kuard2", 8080, 10),
								httpRouteForwardTo("kuard3", 8080, 15),
							),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixrouteHTTPRoute("/",
							&Service{
								Weighted: WeightedService{
									Weight:           5,
									ServiceName:      kuardService.Name,
									ServiceNamespace: kuardService.Namespace,
									ServicePort:      kuardService.Spec.Ports[0],
								},
							},
							&Service{
								Weighted: WeightedService{
									Weight:           10,
									ServiceName:      kuardService2.Name,
									ServiceNamespace: kuardService2.Namespace,
									ServicePort:      kuardService2.Spec.Ports[0],
								},
							},
							&Service{
								Weighted: WeightedService{
									Weight:           15,
									ServiceName:      kuardService3.Name,
									ServiceNamespace: kuardService3.Namespace,
									ServicePort:      kuardService3.Spec.Ports[0],
								},
							},
						)),
					),
				},
			),
		},
		"one service weight zero w/weights for other forwardTos": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				kuardService2,
				kuardService3,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches: httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwards(
								httpRouteForwardTo("kuard", 8080, 5),
								httpRouteForwardTo("kuard2", 8080, 0),
								httpRouteForwardTo("kuard3", 8080, 15),
							),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixrouteHTTPRoute("/",
							&Service{
								Weighted: WeightedService{
									Weight:           5,
									ServiceName:      kuardService.Name,
									ServiceNamespace: kuardService.Namespace,
									ServicePort:      kuardService.Spec.Ports[0],
								},
							},
							&Service{
								Weighted: WeightedService{
									Weight:           0,
									ServiceName:      kuardService2.Name,
									ServiceNamespace: kuardService2.Namespace,
									ServicePort:      kuardService2.Spec.Ports[0],
								},
							},
							&Service{
								Weighted: WeightedService{
									Weight:           15,
									ServiceName:      kuardService3.Name,
									ServiceNamespace: kuardService3.Namespace,
									ServicePort:      kuardService3.Spec.Ports[0],
								},
							},
						)),
					),
				},
			),
		},
		"weight of zero for a single forwardTo results in 503": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				kuardService2,
				kuardService3,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
							ForwardTo: httpRouteForwardTo("kuard", 8080, 0),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", directResponseRouteService("/", http.StatusServiceUnavailable, &Service{
							Weighted: WeightedService{
								Weight:           0,
								ServiceName:      kuardService.Name,
								ServiceNamespace: kuardService.Namespace,
								ServicePort:      kuardService.Spec.Ports[0],
							},
						})),
					),
				},
			),
		},
		"conflicting route match - path": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				kuardService2,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
						CreationTimestamp: metav1.NewTime(time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)),
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
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
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic2",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
						CreationTimestamp: metav1.NewTime(time.Date(1982, time.Month(2), 21, 1, 10, 30, 0, time.UTC)),
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
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
							ForwardTo: httpRouteForwardTo("kuard2", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("test.projectcontour.io",
							prefixrouteHTTPRoute("/blog", service(kuardService2))),
					),
				},
			),
		},
		"conflicting route match - headers": {
			gateway: gatewayWithSelector,
			objs: []interface{}{
				kuardService,
				kuardService2,
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
						CreationTimestamp: metav1.NewTime(time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)),
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
								Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
									Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
									Values: map[string]string{"foo": "bar"},
								},
							}},
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}, {
							Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
								Path: &gatewayapi_v1alpha1.HTTPPathMatch{
									Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
									Value: pointer.StringPtr("/blog"),
								},
							}},
							ForwardTo: httpRouteForwardTo("kuard", 8080, 1),
						}},
					},
				},
				&gatewayapi_v1alpha1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic2",
						Namespace: "projectcontour",
						Labels: map[string]string{
							"app":  "contour",
							"type": "controller",
						},
						CreationTimestamp: metav1.NewTime(time.Date(1982, time.Month(2), 21, 1, 10, 30, 0, time.UTC)),
					},
					Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
						Gateways: &gatewayapi_v1alpha1.RouteGateways{
							Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
						},
						Hostnames: []gatewayapi_v1alpha1.Hostname{
							"test.projectcontour.io",
						},
						Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
							Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
								Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
									Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
									Values: map[string]string{"foo": "bar"},
								},
							}},
							ForwardTo: httpRouteForwardTo("kuard2", 8080, 1),
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(virtualhost("test.projectcontour.io",
						&Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "foo", Value: "bar", MatchType: "exact"},
							},
							Clusters: clustersWeight(service(kuardService2)),
						},
					)),
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			builder := Builder{
				Source: KubernetesCache{
					ConfiguredGateway: types.NamespacedName{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					gateway:     tc.gateway,
					FieldLogger: fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{
						DisablePermitInsecure: tc.disablePermitInsecure,
						FallbackCertificate: &types.NamespacedName{
							Name:      tc.fallbackCertificateName,
							Namespace: tc.fallbackCertificateNamespace,
						},
					},
					&GatewayAPIProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&ListenerProcessor{},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			got := make(map[int]*Listener)
			dag.Visit(listenerMap(got).Visit)

			want := make(map[int]*Listener)
			for _, v := range tc.want {
				if l, ok := v.(*Listener); ok {
					want[l.Port] = l
				}
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestDAGInsert(t *testing.T) {
	// The DAG is sensitive to ordering, adding an ingress, then a service,
	// should have the same result as adding a service, then an ingress.

	sec1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	// Invalid cert in the secret
	sec2 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata("wrong", "wronger"),
	}

	// weird secret with a blank ca.crt that
	// cert manager creates. #1644
	sec3 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			CACertificateKey:    []byte(""),
			v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
			v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
		},
	}

	sec4 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "root",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	fallbackCertificateSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fallbacksecret",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	fallbackCertificateSecretRootNamespace := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fallbacksecret",
			Namespace: "root",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	cert1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca",
			Namespace: "default",
		},
		Data: map[string][]byte{
			CACertificateKey: []byte(fixture.CERTIFICATE),
		},
	}

	i1V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	i1aV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	// i2V1 is functionally identical to i1V1
	i2V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}},
		},
	}

	// i2aV1 is missing a http key from the spec.rule.
	// see issue 606
	i2aV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "test1.test.com",
			}},
		},
	}

	// i3V1 is similar to i2V1 but includes a hostname on the ingress rule
	i3V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "kuard.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}},
		},
	}
	// i4V1 is like i1V1 except it uses a named service port
	i4V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i5V1 is functionally identical to i2V1
	i5V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}
	// i6V1 contains two named vhosts which point to the same service
	// one of those has TLS
	i6V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "a.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}, {
				Host:             "b.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}
	i6aV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "a.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}, {
				Host:             "b.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}

	i6bV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "b.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}

	i6cV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-vhosts",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
				"kubernetes.io/ingress.allow-http":         "false",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host:             "b.example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromString("http"))),
			}},
		},
	}

	// i7V1 contains a single vhost with two paths
	i7V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-paths",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}, {
							Path:    "/kuarder",
							Backend: *backendv1("kuarder", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	// i8V1 is identical to i7V1 but uses multiple IngressRules
	i8V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-rules",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}, {
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/kuarder",
							Backend: *backendv1("kuarder", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}
	// i9V1 is identical to i8V1 but disables non TLS connections
	i9V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-rules",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.allow-http": "false",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}, {
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/kuarder",
							Backend: *backendv1("kuarder", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	i10aV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "two-rules",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/tls-minimum-protocol-version": "1.3",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"b.example.com"},
				SecretName: sec1.Name,
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "b.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i11V1 has a websocket route
	i11V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "websocket",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/websocket-routes": "/ws1 , /ws2",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}, {
							Path:    "/ws1",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i12aV1 has an invalid timeout
	i12aV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/request-timeout": "peanut",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i12bV1 has a reasonable timeout
	i12bV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/request-timeout": "1m30s", // 90 seconds y'all
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	// i12cV1 has an unreasonable timeout
	i12cV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/request-timeout": "infinite",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{HTTP: &networking_v1.HTTPIngressRuleValue{
					Paths: []networking_v1.HTTPIngressPath{{Path: "/",
						Backend: *backendv1("kuard", intstr.FromString("http")),
					}}},
				}}}},
	}

	i12dV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "peanut",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i12eV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "1m30s", // 90 seconds y'all
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i12fV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/response-timeout": "infinite",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{HTTP: &networking_v1.HTTPIngressRuleValue{
					Paths: []networking_v1.HTTPIngressPath{{Path: "/",
						Backend: *backendv1("kuard", intstr.FromString("http")),
					}}},
				}}}},
	}

	// i13_v1 a and b are a pair of ingressesv1 for the same vhost
	// they represent a tricky way over 'overlaying' routes from one
	// ingress onto another
	i13aV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: networking_v1.IngressSpec{
			TLS: []networking_v1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: "example-tls",
			}},
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("app-service", intstr.FromInt(8080)),
						}},
					},
				},
			}},
		},
	}

	i13bV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "challenge", Namespace: "nginx-ingress"},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							Backend: *backendv1("challenge-service", intstr.FromInt(8009)),
						}},
					},
				},
			}},
		},
	}

	i3aV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(80))),
			}},
		},
	}

	i14V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/retry-on":        "gateway-error",
				"projectcontour.io/num-retries":     "6",
				"projectcontour.io/per-try-timeout": "10s",
			},
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i15V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "regex",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "/[^/]+/invoices(/.*|/?)", // issue 1243
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i15InvalidRegexV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "regex",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Path:    "^\\/(?!\\/)(.*?)",
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i16V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wildcards",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				// No hostname.
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}, {
				// Allow wildcard as first label.
				// K8s will only allow hostnames with wildcards of this form.
				Host: "*.example.com",
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}},
					},
				},
			}},
		},
	}

	i17V1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				Host:             "example.com",
				IngressRuleValue: ingressrulev1value(backendv1("kuard", intstr.FromInt(8080))),
			}},
		},
	}

	iPathMatchTypesV1 := &networking_v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pathmatchtypes",
			Namespace: "default",
		},
		Spec: networking_v1.IngressSpec{
			Rules: []networking_v1.IngressRule{{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{
							{
								PathType: (*networking_v1.PathType)(pointer.StringPtr("Exact")),
								Path:     "/exact",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(pointer.StringPtr("Exact")),
								Path:     "/exact_with_regex/.*",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(pointer.StringPtr("Prefix")),
								Path:     "/prefix",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(pointer.StringPtr("Prefix")),
								Path:     "/prefix_trailing_slash/",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(pointer.StringPtr("Prefix")),
								Path:     "/prefix_with_regex/.*",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(pointer.StringPtr("ImplementationSpecific")),
								Path:     "/implementation_specific",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
							{
								PathType: (*networking_v1.PathType)(pointer.StringPtr("ImplementationSpecific")),
								Path:     "/implementation_specific_with_regex/.*",
								Backend:  *backendv1("kuard", intstr.FromString("http")),
							},
						},
					},
				},
			}},
		},
	}

	// s3a and b have http/2 protocol annotations
	s3a := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.h2c": "80,http",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8888),
			}},
		},
	}

	s3b := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s3a.Name,
			Namespace: s3a.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.h2": "80,http",
			},
		},
		Spec: s3a.Spec,
	}

	s3c := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s3b.Name,
			Namespace: s3b.Namespace,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "80,http",
			},
		},
		Spec: s3b.Spec,
	}

	sec13 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: v1.SecretTypeTLS,
		Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
	}

	s13a := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-service",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s13b := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "challenge-service",
			Namespace: "nginx-ingress",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8009,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	proxyMultipleBackends := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}, {
					Name: "kuarder",
					Port: 8080,
				}},
			}},
		},
	}

	proxyMinTLS12 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_api_v1.TLS{
					SecretName:             sec1.Name,
					MinimumProtocolVersion: "1.2",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyMinTLS13 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_api_v1.TLS{
					SecretName:             sec1.Name,
					MinimumProtocolVersion: "1.3",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyMinTLSInvalid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_api_v1.TLS{
					SecretName:             sec1.Name,
					MinimumProtocolVersion: "0.999",
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyWeightsTwoRoutesDiffWeights := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   8080,
					Weight: 90,
				}},
			}, {
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/b",
				}},
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   8080,
					Weight: 60,
				}},
			}},
		},
	}

	proxyWeightsOneRouteDiffWeights := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/a",
				}},
				Services: []contour_api_v1.Service{{
					Name:   "kuard",
					Port:   8080,
					Weight: 90,
				}, {
					Name:   "kuard",
					Port:   8080,
					Weight: 60,
				}},
			}},
		},
	}

	proxyRetryPolicyValidTimeout := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				RetryPolicy: &contour_api_v1.RetryPolicy{
					NumRetries:    6,
					PerTryTimeout: "10s",
				},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyRetryPolicyInvalidTimeout := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				RetryPolicy: &contour_api_v1.RetryPolicy{
					NumRetries:    6,
					PerTryTimeout: "please",
				},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyRetryPolicyZeroRetries := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				RetryPolicy: &contour_api_v1.RetryPolicy{
					NumRetries:    0,
					PerTryTimeout: "10s",
				},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTimeoutPolicyInvalidResponse := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
					Response: "peanut",
				},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTimeoutPolicyValidResponse := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
					Response: "1m30s",
				},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxyTimeoutPolicyInfiniteResponse := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "bar.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				TimeoutPolicy: &contour_api_v1.TimeoutPolicy{
					Response: "infinite",
				},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	// s1a carries the tls annotation
	s1a := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "8080",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	// s1b carries all four ingress annotations{
	s1b := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
			Annotations: map[string]string{
				"projectcontour.io/max-connections":      "9000",
				"projectcontour.io/max-pending-requests": "4096",
				"projectcontour.io/max-requests":         "404",
				"projectcontour.io/max-retries":          "7",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	// s2 is like s1 but with a different name
	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	// s2a is like s1 but with a different name again.
	// used in testing override priority.
	s2a := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuardest",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	// s3 is like s1 but has a different port
	s3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       9999,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s4 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: "marketing",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s9 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     80,
			}},
		},
	}

	s10 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-passthrough",
			Namespace: "default",
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

	s11 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blog",
			Namespace: "it",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "blog",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	s12 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "teama",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s13 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "teamb",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	s14 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			ExternalName: "externalservice.io",
			Ports: []v1.ServicePort{{
				Protocol: "TCP",
				Port:     80,
			}},
			Type: v1.ServiceTypeExternalName,
		},
	}

	proxyDelegatedTLSSecret := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-tls-delegation",
			Namespace: s10.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "app-with-tls-delegation.127.0.0.1.nip.io",
				TLS: &contour_api_v1.TLS{
					SecretName: "projectcontour/ssl-cert", // not delegated
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s10.Name,
					Port: 80,
				}},
			}},
		},
	}

	proxy1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy1a tcp forwards traffic to default/kuard:8080 by TLS pass-through it.
	proxy1a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			},
		},
	}

	// proxy1b is a straight HTTP forward, no conditions.
	proxy1b := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
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

	// proxy1c is a straight forward, with prefix and header conditions.
	proxy1c := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:    "x-request-id",
						Present: true,
					},
				}, {
					Prefix: "/kuard",
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "e-tag",
						Contains: "abcdef",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:        "x-timeout",
						NotContains: "infinity",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "digest-auth",
						Exact: "scott",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "digest-password",
						NotExact: "tiger",
					},
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy1d tcp forwards secure traffic to default/kuard:8080 by TLS pass-through it,
	// insecure traffic is 301 upgraded.
	proxy1d := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// proxy1e tcp forwards secure traffic to default/kuard:8080 by TLS pass-through it,
	// insecure traffic is not 301 upgraded because of the permitInsecure: true annotation.
	proxy1e := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-tcp",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "kuard.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			Routes: []contour_api_v1.Route{{
				PermitInsecure: true,
				Services: []contour_api_v1.Service{{
					Name: s10.Name,
					Port: 80,
				}},
			}},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: s10.Name,
					Port: 443,
				}},
			},
		},
	}

	//proxy1f is identical to proxy1 and ir1, except for a different service.
	// Used to test priority when importing ir then httproxy.
	proxy1f := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s2a.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy2a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "kubesystem",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Conditions: []contour_api_v1.MatchCondition{{
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:    "x-request-id",
						Present: true,
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:        "x-timeout",
						NotContains: "infinity",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:  "digest-auth",
						Exact: "scott",
					},
				}},
				Name:      "kuard",
				Namespace: "default",
			}},
		},
	}

	proxy2b := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/kuard",
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "e-tag",
						Contains: "abcdef",
					},
				}, {
					Header: &contour_api_v1.HeaderMatchCondition{
						Name:     "digest-password",
						NotExact: "tiger",
					},
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxy2c := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				HealthCheckPolicy: &contour_api_v1.HTTPHealthCheckPolicy{
					Path: "/healthz",
				},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy2d is a proxy with two routes that have the same prefix and a Contains header
	// condition on the same header, differing only in the value of the condition.
	proxy2d := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Header: &contour_api_v1.HeaderMatchCondition{
								Name:     "e-tag",
								Contains: "abc",
							},
						},
						{
							Prefix: "/",
						},
					},
					Services: []contour_api_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Header: &contour_api_v1.HeaderMatchCondition{
								Name:     "e-tag",
								Contains: "def",
							},
						},
						{
							Prefix: "/",
						},
					},
					Services: []contour_api_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
			},
		},
	}

	// proxy2e is a proxy with two routes that both have a condition on the same
	// header, one using Contains and one using NotContains.
	proxy2e := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Header: &contour_api_v1.HeaderMatchCondition{
								Name:     "e-tag",
								Contains: "abc",
							},
						},
						{
							Prefix: "/",
						},
					},
					Services: []contour_api_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
				{
					Conditions: []contour_api_v1.MatchCondition{
						{
							Header: &contour_api_v1.HeaderMatchCondition{
								Name:        "e-tag",
								NotContains: "abc",
							},
						},
						{
							Prefix: "/",
						},
					},
					Services: []contour_api_v1.Service{{
						Name: "kuard",
						Port: 8080,
					}},
				},
			},
		},
	}

	// proxy6 has TLS and does not specify min tls version
	proxy6 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "foo.com",
				TLS: &contour_api_v1.TLS{
					SecretName: sec1.Name,
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	proxy17 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
					UpstreamValidation: &contour_api_v1.UpstreamValidation{
						CACertificate: cert1.Name,
						SubjectName:   "example.com",
					},
				}},
			}},
		},
	}
	protocolh2 := "h2"
	proxy17h2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name:     "kuard",
					Port:     8080,
					Protocol: &protocolh2,
					UpstreamValidation: &contour_api_v1.UpstreamValidation{
						CACertificate: cert1.Name,
						SubjectName:   "example.com",
					},
				}},
			}},
		},
	}

	// proxy18 is downstream validation, HTTP route
	proxy18 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						CACertificate: cert1.Name,
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy19 is downstream validation, TCP proxying
	proxy19 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						CACertificate: cert1.Name,
					},
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// proxy10 has a websocket route
	proxy10 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/websocket",
				}},
				EnableWebsockets: true,
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy10b has a websocket route w/multiple upstreams
	proxy10b := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}, {
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/websocket",
				}},
				EnableWebsockets: true,
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy12 tests mirroring
	proxy12 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}, {
					Name:   s2.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	// proxy13 has two mirrors, invalid.
	proxy13 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}, {
					Name:   s2.Name,
					Port:   8080,
					Mirror: true,
				}, {
					// it is legal to mention a service more that
					// once, however it is not legal for more than one
					// service to be marked as mirror.
					Name:   s2.Name,
					Port:   8080,
					Mirror: true,
				}},
			}},
		},
	}

	// invalid because tcpproxy both includes another and
	// has a list of services.
	proxy37 := &contour_api_v1.HTTPProxy{
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
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// Invalid because tcpproxy neither includes another httpproxy
	// nor has a list of services.
	proxy37a := &contour_api_v1.HTTPProxy{
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

	// proxy38 is invalid when combined with proxy39
	// as the latter is a root.
	proxy38 := &contour_api_v1.HTTPProxy{
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
					Namespace: s1.Namespace,
				},
			},
		},
	}

	proxy39 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: s1.Namespace,
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
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// proxy39broot is a valid TCPProxy which includes to another TCPProxy
	proxy39broot := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Include: &contour_api_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: s1.Namespace,
				},
			},
		},
	}

	proxy39brootplural := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_api_v1.TLS{
					Passthrough: true,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				IncludesDeprecated: &contour_api_v1.TCPProxyInclude{
					Name:      "foo",
					Namespace: s1.Namespace,
				},
			},
		},
	}

	proxy39bchild := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	proxy40 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			},
		},
	}

	// proxy20 is downstream validation, skip cert validation
	proxy20 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: sec1.Name,
					ClientValidation: &contour_api_v1.DownstreamValidation{
						SkipClientCertValidation: true,
					},
				},
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// issue 2309, each route must have at least one service
	proxy41 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-service",
			Namespace: s1.Namespace,
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

	proxy100 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "marketingwww",
				Namespace: "marketing",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/blog",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy100a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marketingwww",
			Namespace: "marketing",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	proxy100b := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marketingwww",
			Namespace: "marketing",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/infotech",
				}},
				Services: []contour_api_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	proxy100c := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marketingwww",
			Namespace: "marketing",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Includes: []contour_api_v1.Include{{
				Name:      "marketingit",
				Namespace: "it",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/it",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/infotech",
				}},
				Services: []contour_api_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}, {
				Services: []contour_api_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	proxy100d := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marketingit",
			Namespace: "it",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/foo",
				}},
				Services: []contour_api_v1.Service{{
					Name: "blog",
					Port: 8080,
				}},
			}},
		},
	}

	// proxy101 and proxy101a test inclusion without a specified namespace.
	proxy101 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy101a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy101.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// invalid because two prefix conditions on route.
	proxy102 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/v1",
				}, {
					Prefix: "/api",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// invalid because two prefix conditions on include.
	proxy103 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name:      "www",
				Namespace: "teama",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/v1",
				}, {
					Prefix: "/api",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy103a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "www",
			Namespace: "teama",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/v1",
				}, {
					Prefix: "/api",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy104 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy104a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy104.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy105 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy105a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy105.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy106 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/kuarder/",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy106a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy105.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy107 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: s1.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contour_api_v1.Include{{
				Name: "kuarder",
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/kuarder",
				}},
			}},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy107a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuarder",
			Namespace: proxy105.Namespace,
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/withavengeance",
				}},
				Services: []contour_api_v1.Service{{
					Name: s2.Name,
					Port: 8080,
				}},
			}},
		},
	}

	// proxy108 and proxy108a test duplicate conditions on include
	proxy108 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root",
			Namespace: s1.Namespace,
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
				Name:      "blogteama",
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
					Name: s1.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy108a := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blogteama",
			Namespace: "teama",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s12.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxy108b := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blogteamb",
			Namespace: "teamb",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: s13.Name,
					Port: 8080,
				}},
			}},
		},
	}

	proxyReplaceHostHeaderRoute := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "bar.com",
					}},
				},
			}},
		},
	}

	proxyReplaceHostHeaderService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "nginx",
					Port: 80,
					RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
						Set: []contour_api_v1.HeaderValue{{
							Name:  "Host",
							Value: "bar.com",
						}},
					},
				}},
			}},
		},
	}

	proxyReplaceHostHeaderMultiple := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "bar.com",
					}, {
						Name:  "x-header",
						Value: "bar.com",
					}, {
						Name:  "y-header",
						Value: "zed.com",
					}},
				},
			}},
		},
	}

	proxyReplaceNonHostHeader := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "x-header",
						Value: "bar.com",
					}},
				},
			}},
		},
	}

	proxyReplaceHeaderEmptyValue := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name: "x-header",
					}},
				},
			}},
		},
	}

	proxyCookieLoadBalancer := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: "Cookie",
				},
			}},
		},
	}

	proxyLoadBalancerHashPolicyHeader := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
					RequestHashPolicies: []contour_api_v1.RequestHashPolicy{
						{
							Terminal: true,
							HeaderHashOptions: &contour_api_v1.HeaderHashOptions{
								HeaderName: "X-Some-Header",
							},
						},
						{
							// Lower case but duplicated, should be ignored.
							HeaderHashOptions: &contour_api_v1.HeaderHashOptions{
								HeaderName: "x-some-header",
							},
						},
						{
							HeaderHashOptions: nil,
						},
						{
							HeaderHashOptions: &contour_api_v1.HeaderHashOptions{
								HeaderName: "X-Some-Other-Header",
							},
						},
						{
							HeaderHashOptions: &contour_api_v1.HeaderHashOptions{
								HeaderName: "",
							},
						},
					},
				},
			}},
		},
	}

	proxyLoadBalancerHashPolicyHeaderAllInvalid := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "nginx",
					Port: 80,
				}},
				LoadBalancerPolicy: &contour_api_v1.LoadBalancerPolicy{
					Strategy: "RequestHash",
					RequestHashPolicies: []contour_api_v1.RequestHashPolicy{
						{
							HeaderHashOptions: nil,
						},
						{
							HeaderHashOptions: &contour_api_v1.HeaderHashOptions{
								HeaderName: "",
							},
						},
					},
				},
			}},
		},
	}

	// proxy109 has a route that rewrites headers.
	proxy109 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				RequestHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "In-Foo",
						Value: "bar",
					}},
					Remove: []string{
						"In-Baz",
					},
				},
				ResponseHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Out-Foo",
						Value: "bar",
					}},
					Remove: []string{
						"Out-Baz",
					},
				},
			}},
		},
	}
	// proxy111 has a route that rewrites headers.
	proxy111 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
				ResponseHeadersPolicy: &contour_api_v1.HeadersPolicy{
					Set: []contour_api_v1.HeaderValue{{
						Name:  "Host",
						Value: "bar.baz",
					}},
				},
			}},
		},
	}
	// proxy112 has a route that rewrites headers.
	proxy112 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
					ResponseHeadersPolicy: &contour_api_v1.HeadersPolicy{
						Set: []contour_api_v1.HeaderValue{{
							Name:  "Host",
							Value: "bar.baz",
						}},
					},
				}},
			}},
		},
	}
	protocol := "h2c"
	proxy110 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name:     "kuard",
					Port:     8080,
					Protocol: &protocol,
				}},
			}},
		},
	}

	proxyExternalNameService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
			},
			Routes: []contour_api_v1.Route{{
				Conditions: []contour_api_v1.MatchCondition{{
					Prefix: "/",
				}},
				Services: []contour_api_v1.Service{{
					Name: s14.GetName(),
					Port: 80,
				}},
			}},
		},
	}

	tcpProxyExternalNameService := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example.com",
				TLS: &contour_api_v1.TLS{
					SecretName: sec1.Name,
				},
			},
			TCPProxy: &contour_api_v1.TCPProxy{
				Services: []contour_api_v1.Service{{
					Name:     s14.GetName(),
					Port:     80,
					Protocol: pointer.StringPtr("tls"),
				}},
			},
		},
	}

	tests := map[string]struct {
		objs                         []interface{}
		disablePermitInsecure        bool
		fallbackCertificateName      string
		fallbackCertificateNamespace string
		want                         []Vertex
	}{
		"ingressv1: insert ingress w/ default backend w/o matching service": {
			objs: []interface{}{
				i1V1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ default backend": {
			objs: []interface{}{
				i1V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ single unnamed backend w/o matching service": {
			objs: []interface{}{
				i2V1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ single unnamed backend": {
			objs: []interface{}{
				i2V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress with missing spec.rule.http key": {
			objs: []interface{}{
				i2aV1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ host name and single backend w/o matching service": {
			objs: []interface{}{
				i3V1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ host name and single backend": {
			objs: []interface{}{
				i3V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert non matching service then ingress w/ default backend": {
			objs: []interface{}{
				s2,
				i1V1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ default backend then matching service with wrong port": {
			objs: []interface{}{
				i1V1,
				s3,
			},
			want: listeners(),
		},
		"ingressv1: insert unnamed ingress w/ single backend then matching service with wrong port": {
			objs: []interface{}{
				i2V1,
				s3,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ default backend then matching service w/ named port": {
			objs: []interface{}{
				i4V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service w/ named port then ingress w/ default backend": {
			objs: []interface{}{
				s1,
				i4V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ single unnamed backend w/ named service port then service": {
			objs: []interface{}{
				i5V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service then ingress w/ single unnamed backend w/ named service port": {
			objs: []interface{}{
				s1,
				i5V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert secret": {
			objs: []interface{}{
				sec1,
			},
			want: listeners(),
		},
		"ingressv1: insert secret then ingress w/o tls": {
			objs: []interface{}{
				sec1,
				i1V1,
			},
			want: listeners(),
		},
		"ingressv1: insert service, secret then ingress w/o tls": {
			objs: []interface{}{
				s1,
				sec1,
				i1V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert secret then ingress w/ tls": {
			objs: []interface{}{
				sec1,
				i3V1,
			},
			want: listeners(),
		},
		"ingressv1: insert service, secret then ingress w/ tls": {
			objs: []interface{}{
				s1,
				sec1,
				i3V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("kuard.example.com", sec1, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service w/ secret with w/ blank ca.crt": {
			objs: []interface{}{
				s1,
				sec3, // issue 1644
				i3V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("kuard.example.com", sec3, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert invalid secret then ingress w/o tls": {
			objs: []interface{}{
				sec2,
				i1V1,
			},
			want: listeners(),
		},
		"ingressv1: insert service, invalid secret then ingress w/o tls": {
			objs: []interface{}{
				s1,
				sec2,
				i1V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert invalid secret then ingress w/ tls": {
			objs: []interface{}{
				sec2,
				i3V1,
			},
			want: listeners(),
		},
		"ingressv1: insert service, invalid secret then ingress w/ tls": {
			objs: []interface{}{
				s1,
				sec2,
				i3V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two vhosts": {
			objs: []interface{}{
				i6V1,
			},
			want: nil, // no matching service
		},
		"ingressv1: insert ingress w/ two vhosts then matching service": {
			objs: []interface{}{
				i6V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service then ingress w/ two vhosts": {
			objs: []interface{}{
				s1,
				i6V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two vhosts then service then secret": {
			objs: []interface{}{
				i6V1,
				s1,
				sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert service then secret then ingress w/ two vhosts": {
			objs: []interface{}{
				s1,
				sec1,
				i6V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("a.example.com", prefixroute("/", service(s1))),
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two paths then one service": {
			objs: []interface{}{
				i7V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com",
							prefixroute("/", service(s1)),
						),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two paths then services": {
			objs: []interface{}{
				i7V1,
				s2,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com",
							prefixroute("/", service(s1)),
							prefixroute("/kuarder", service(s2)),
						),
					),
				},
			),
		},
		"ingressv1: insert two services then ingress w/ two ingress rules": {
			objs: []interface{}{
				s1, s2, i8V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com",
							prefixroute("/", service(s1)),
							prefixroute("/kuarder", service(s2)),
						),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ two paths httpAllowed: false": {
			objs: []interface{}{
				i9V1,
			},
			want: []Vertex{},
		},
		"ingressv1: insert ingress w/ two paths httpAllowed: false then tls and service": {
			objs: []interface{}{
				i9V1,
				sec1,
				s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1,
							prefixroute("/", service(s1)),
							prefixroute("/kuarder", service(s2)),
						),
					),
				},
			),
		},
		"ingressv1: insert default ingress httpAllowed: false": {
			objs: []interface{}{
				i1aV1,
			},
			want: []Vertex{},
		},
		"ingressv1: insert default ingress httpAllowed: false then tls and service": {
			objs: []interface{}{
				i1aV1, sec1, s1,
			},
			want: []Vertex{}, // default ingress cannot be tls
		},
		"ingressv1: insert ingress w/ two vhosts httpAllowed: false": {
			objs: []interface{}{
				i6aV1,
			},
			want: []Vertex{},
		},
		"ingressv1: insert ingress w/ two vhosts httpAllowed: false then tls and service": {
			objs: []interface{}{
				i6aV1, sec1, s1,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, prefixroute("/", service(s1))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ force-ssl-redirect: true": {
			objs: []interface{}{
				i6bV1, sec1, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, routeUpgrade("/", service(s1))),
					),
				},
			),
		},

		"ingressv1: insert ingress w/ force-ssl-redirect: true and allow-http: false": {
			objs: []interface{}{
				i6cV1, sec1, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("b.example.com", sec1, routeUpgrade("/", service(s1))),
					),
				},
			),
		},
		"insert httpproxy with tls version 1.2": {
			objs: []interface{}{
				proxyMinTLS12, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "foo.com",
								ListenerName: "ingress_https",
								routes: routes(
									routeUpgrade("/", service(s1)),
								),
							},
							MinTLSVersion: "1.2",
							Secret:        secret(sec1),
						},
					),
				},
			),
		},
		"insert httpproxy with tls version 1.3": {
			objs: []interface{}{
				proxyMinTLS13, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "foo.com",
								ListenerName: "ingress_https",
								routes: routes(
									routeUpgrade("/", service(s1)),
								),
							},
							MinTLSVersion: "1.3",
							Secret:        secret(sec1),
						},
					),
				},
			),
		},
		"insert httpproxy with invalid tls version": {
			objs: []interface{}{
				proxyMinTLSInvalid, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", service(s1))),
					),
				},
			),
		},
		"insert httpproxy referencing two backends, one missing": {
			objs: []interface{}{
				proxyMultipleBackends, s2,
			},
			want: listeners(),
		},
		"insert httpproxy referencing two backends": {
			objs: []interface{}{
				proxyMultipleBackends, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s1), service(s2))),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ tls min proto annotation": {
			objs: []interface{}{
				i10aV1,
				sec1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("b.example.com", prefixroute("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "b.example.com",
								ListenerName: "ingress_https",
								routes: routes(
									prefixroute("/", service(s1)),
								),
							},
							MinTLSVersion: "1.3",
							Secret:        secret(sec1),
						},
					),
				},
			),
		},
		"ingressv1: insert ingress w/ websocket route annotation": {
			objs: []interface{}{
				i11V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", service(s1)),
							routeWebsocket("/ws1", service(s1)),
						),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ invalid legacy timeout annotation": {
			objs: []interface{}{
				i12aV1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ invalid timeout annotation": {
			objs: []interface{}{
				i12dV1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
						}),
					),
				},
			),
		},
		"insert httpproxy w/ invalid timeoutpolicy": {
			objs: []interface{}{
				proxyTimeoutPolicyInvalidResponse,
				s1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress w/ valid legacy timeout annotation": {
			objs: []interface{}{
				i12bV1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: TimeoutPolicy{
								ResponseTimeout: timeout.DurationSetting(90 * time.Second),
							},
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ valid timeout annotation": {
			objs: []interface{}{
				i12eV1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: TimeoutPolicy{
								ResponseTimeout: timeout.DurationSetting(90 * time.Second),
							},
						}),
					),
				},
			),
		},
		"insert httpproxy w/ valid timeoutpolicy": {
			objs: []interface{}{
				proxyTimeoutPolicyValidResponse,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: TimeoutPolicy{
								ResponseTimeout: timeout.DurationSetting(90 * time.Second),
							},
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ legacy infinite timeout annotation": {
			objs: []interface{}{
				i12cV1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: TimeoutPolicy{
								ResponseTimeout: timeout.DisabledSetting(),
							},
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress w/ infinite timeout annotation": {
			objs: []interface{}{
				i12fV1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: TimeoutPolicy{
								ResponseTimeout: timeout.DisabledSetting(),
							},
						}),
					),
				},
			),
		},
		"insert httpproxy w/ infinite timeoutpolicy": {
			objs: []interface{}{
				proxyTimeoutPolicyInfiniteResponse,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							TimeoutPolicy: TimeoutPolicy{
								ResponseTimeout: timeout.DisabledSetting(),
							},
						}),
					),
				},
			),
		},
		"insert httpproxy with missing tls delegation should not present port 80": {
			objs: []interface{}{
				s10, proxyDelegatedTLSSecret,
			},
			want: listeners(), // no listeners, ir19 is invalid
		},
		"insert httpproxy with retry annotations": {
			objs: []interface{}{
				proxyRetryPolicyValidTimeout,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "5xx",
								NumRetries:    6,
								PerTryTimeout: timeout.DurationSetting(10 * time.Second),
							},
						}),
					),
				},
			),
		},
		"insert httpproxy with invalid PerTryTimeout": {
			objs: []interface{}{
				proxyRetryPolicyInvalidTimeout,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "5xx",
								NumRetries:    6,
								PerTryTimeout: timeout.DefaultSetting(),
							},
						}),
					),
				},
			),
		},
		"insert httpproxy with zero retry count": {
			objs: []interface{}{
				proxyRetryPolicyZeroRetries,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("bar.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "5xx",
								NumRetries:    1,
								PerTryTimeout: timeout.DurationSetting(10 * time.Second),
							},
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress with timeout policy": {
			objs: []interface{}{
				i14V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s1),
							RetryPolicy: &RetryPolicy{
								RetryOn:       "gateway-error",
								NumRetries:    6,
								PerTryTimeout: timeout.DurationSetting(10 * time.Second),
							},
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress with regex route": {
			objs: []interface{}{
				i15V1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", &Route{
							PathMatchCondition: regex("/[^/]+/invoices(/.*|/?)"),
							Clusters:           clustermap(s1),
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress with invalid regex route": {
			objs: []interface{}{
				i15InvalidRegexV1,
				s1,
			},
			want: listeners(),
		},
		"ingressv1: insert ingress with various path match types": {
			objs: []interface{}{
				iPathMatchTypesV1,
				s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							&Route{
								PathMatchCondition: exact("/exact"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: exact("/exact_with_regex/.*"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: prefixSegment("/prefix"),
								Clusters:           clustermap(s1),
							},
							&Route{
								// Trailing slash is stripped.
								PathMatchCondition: prefixSegment("/prefix_trailing_slash"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: prefixSegment("/prefix_with_regex/.*"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: prefixString("/implementation_specific"),
								Clusters:           clustermap(s1),
							},
							&Route{
								PathMatchCondition: regex("/implementation_specific_with_regex/.*"),
								Clusters:           clustermap(s1),
							},
						),
					),
				},
			),
		},
		"ingressv1: insert ingress with wildcard hostnames": {
			objs: []interface{}{
				s1,
				i16V1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*", prefixroute("/", service(s1))),
						virtualhost("*.example.com", &Route{
							PathMatchCondition: &PrefixMatchCondition{Prefix: "/"},
							HeaderMatchConditions: []HeaderMatchCondition{
								{
									Name:      ":authority",
									MatchType: HeaderMatchTypeRegex,
									Value:     "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.example\\.com",
								},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"ingressv1: insert ingress overlay": {
			objs: []interface{}{
				i13aV1, i13bV1, sec13, s13a, s13b,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeUpgrade("/", service(s13a)),
							prefixroute("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", service(s13b)),
						),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("example.com", sec13,
							routeUpgrade("/", service(s13a)),
							prefixroute("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk", service(s13b)),
						),
					),
				},
			),
		},
		"ingressv1: h2c service annotation": {
			objs: []interface{}{
				i3aV1, s3a,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Protocol: "h2c",
								Weighted: WeightedService{
									Weight:           1,
									ServiceName:      s3a.Name,
									ServiceNamespace: s3a.Namespace,
									ServicePort:      s3a.Spec.Ports[0],
								},
							}),
						),
					),
				},
			),
		},
		"ingressv1: h2 service annotation": {
			objs: []interface{}{
				i3aV1, s3b,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Protocol: "h2",
								Weighted: WeightedService{
									Weight:           1,
									ServiceName:      s3b.Name,
									ServiceNamespace: s3b.Namespace,
									ServicePort:      s3b.Spec.Ports[0],
								},
							}),
						),
					),
				},
			),
		},
		"ingressv1: tls service annotation": {
			objs: []interface{}{
				i3aV1, s3c,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Protocol: "tls",
								Weighted: WeightedService{
									Weight:           1,
									ServiceName:      s3c.Name,
									ServiceNamespace: s3c.Namespace,
									ServicePort:      s3c.Spec.Ports[0],
								},
							}),
						),
					),
				},
			),
		},
		"ingressv1: insert ingress then service w/ upstream annotations": {
			objs: []interface{}{
				i1V1,
				s1b,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("*",
							prefixroute("/", &Service{
								Weighted: WeightedService{
									Weight:           1,
									ServiceName:      s1b.Name,
									ServiceNamespace: s1b.Namespace,
									ServicePort:      s1b.Spec.Ports[0],
								},
								MaxConnections:     9000,
								MaxPendingRequests: 4096,
								MaxRequests:        404,
								MaxRetries:         7,
							}),
						),
					),
				},
			),
		},
		"insert httpproxy with two routes to the same service": {
			objs: []interface{}{
				proxyWeightsTwoRoutesDiffWeights, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/a", &Cluster{
								Upstream: &Service{
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s1.Name,
										ServiceNamespace: s1.Namespace,
										ServicePort:      s1.Spec.Ports[0],
									},
								},
								Weight: 90,
							}),
							routeCluster("/b", &Cluster{
								Upstream: &Service{
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s1.Name,
										ServiceNamespace: s1.Namespace,
										ServicePort:      s1.Spec.Ports[0],
									},
								},
								Weight: 60,
							}),
						),
					),
				},
			),
		},
		"insert httpproxy with one routes to the same service with two different weights": {
			objs: []interface{}{
				proxyWeightsOneRouteDiffWeights, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/a",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
									Weight: 90,
								}, &Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
									Weight: 60,
								},
							),
						),
					),
				},
			),
		},
		"insert httproxy": {
			objs: []interface{}{
				proxy1, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert httproxy w/o condition": {
			objs: []interface{}{
				proxy1b, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s1))),
					),
				},
			),
		},
		"insert httproxy with invalid include": {
			objs: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "example-com",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
						},
						Includes: []contour_api_v1.Include{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/finance",
							}},
							Name:      "non-existent",
							Namespace: "non-existent",
						}},
					},
				},
			},
			want: nil, // no listener created
		},
		"insert httproxy w/ conditions": {
			objs: []interface{}{
				proxy1c, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/kuard"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "x-request-id", MatchType: "present"},
								{Name: "e-tag", Value: "abcdef", MatchType: "contains"},
								{Name: "x-timeout", Value: "infinity", MatchType: "contains", Invert: true},
								{Name: "digest-auth", Value: "scott", MatchType: "exact"},
								{Name: "digest-password", Value: "tiger", MatchType: "exact", Invert: true},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"insert httproxy w/ multiple routes with a Contains condition on the same header": {
			objs: []interface{}{
				proxy2d, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "e-tag", Value: "abc", MatchType: "contains"},
							},
							Clusters: clusters(service(s1)),
						}, &Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "e-tag", Value: "def", MatchType: "contains"},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"insert httproxy w/ multiple routes with condition on the same header, one Contains and one NotContains": {
			objs: []interface{}{
				proxy2e, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "e-tag", Value: "abc", MatchType: "contains"},
							},
							Clusters: clusters(service(s1)),
						}, &Route{
							PathMatchCondition: prefixString("/"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "e-tag", Value: "abc", MatchType: "contains", Invert: true},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"insert httproxy w/ included conditions": {
			objs: []interface{}{
				proxy2a, proxy2b, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/kuard"),
							HeaderMatchConditions: []HeaderMatchCondition{
								{Name: "x-request-id", MatchType: "present"},
								{Name: "x-timeout", Value: "infinity", MatchType: "contains", Invert: true},
								{Name: "digest-auth", Value: "scott", MatchType: "exact"},
								{Name: "e-tag", Value: "abcdef", MatchType: "contains"},
								{Name: "digest-password", Value: "tiger", MatchType: "exact", Invert: true},
							},
							Clusters: clusters(service(s1)),
						}),
					),
				},
			),
		},
		"insert httpproxy w/ healthcheck": {
			objs: []interface{}{
				proxy2c, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/", &Cluster{
								Upstream: service(s1),
								HTTPHealthCheckPolicy: &HTTPHealthCheckPolicy{
									Path: "/healthz",
								},
							}),
						),
					),
				},
			),
		},
		"insert httpproxy with mirroring route": {
			objs: []interface{}{
				proxy12, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							withMirror(prefixroute("/", service(s1)), service(s2)),
						),
					),
				},
			),
		},
		"insert httpproxy with two mirrors": {
			objs: []interface{}{
				proxy13, s1, s2,
			},
			want: listeners(),
		},
		"insert httpproxy with websocket route and prefix rewrite": {
			objs: []interface{}{
				proxy10, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", service(s1)),
							routeWebsocket("/websocket", service(s1)),
						),
					),
				},
			),
		},
		"insert httpproxy with multiple upstreams prefix rewrite route and websockets along one path": {
			objs: []interface{}{
				proxy10b, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", service(s1)),
							routeWebsocket("/websocket", service(s1)),
						),
					),
				},
			),
		},

		"insert httpproxy with protocol and service": {
			objs: []interface{}{
				proxy110, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeProtocol("/", protocol, service(s1))),
					),
				},
			),
		},

		"insert httpproxy without tls version": {
			objs: []interface{}{
				proxy6, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("foo.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						securevirtualhost("foo.com", sec1, routeUpgrade("/", service(s1))),
					),
				},
			),
		},
		"insert httpproxy expecting upstream verification": {
			objs: []interface{}{
				cert1, proxy17, s1a,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Protocol: "tls",
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1a.Name,
											ServiceNamespace: s1a.Namespace,
											ServicePort:      s1a.Spec.Ports[0],
										},
									},
									Protocol: "tls",
									UpstreamValidation: &PeerValidationContext{
										CACertificate: secret(cert1),
										SubjectName:   "example.com",
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with h2 expecting upstream verification": {
			objs: []interface{}{
				cert1, proxy17h2, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1a.Name,
											ServiceNamespace: s1a.Namespace,
											ServicePort:      s1a.Spec.Ports[0],
										},
									},
									Protocol: "h2",
									UpstreamValidation: &PeerValidationContext{
										CACertificate: secret(cert1),
										SubjectName:   "example.com",
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy expecting upstream verification, no certificate": {
			objs: []interface{}{
				proxy17, s1a,
			},
			want: listeners(), //no listeners, missing certificate
		},
		"insert httpproxy expecting upstream verification, no annotation on service": {
			objs: []interface{}{
				cert1, proxy17, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							prefixroute("/", service(s1)),
						),
					),
				},
			),
		},
		"insert httpproxy with downstream verification": {
			objs: []interface{}{
				cert1, proxy18, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
								routes: routes(
									routeUpgrade("/", service(s1))),
							},
							MinTLSVersion: "1.2",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								CACertificate: &Secret{Object: cert1},
							},
						},
					),
				},
			),
		},
		"insert httpproxy w/ tcpproxy in tls termination mode w/ downstream verification": {
			objs: []interface{}{
				cert1, proxy19, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
							MinTLSVersion: "1.2",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								CACertificate: &Secret{Object: cert1},
							},
						},
					),
				},
			),
		},
		"insert httpproxy w/ tls termination mode w/ skip cert verification": {
			objs: []interface{}{
				proxy20, s1, sec1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s1))),
					),
				}, &Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
								routes: routes(
									routeUpgrade("/", service(s1))),
							},
							MinTLSVersion: "1.2",
							Secret:        secret(sec1),
							DownstreamValidation: &PeerValidationContext{
								SkipClientCertValidation: true,
							},
						},
					),
				},
			),
		},
		"insert httpproxy with downstream verification, missing ca certificate": {
			objs: []interface{}{
				proxy18, s1, sec1,
			},
			want: listeners(),
		},
		"insert httpproxy with invalid tcpproxy": {
			objs: []interface{}{proxy37, s1},
			want: listeners(),
		},
		"insert httpproxy with empty tcpproxy": {
			objs: []interface{}{proxy37a, s1},
			want: listeners(),
		},
		"insert httpproxy w/ tcpproxy w/ missing include": {
			objs: []interface{}{proxy38, s1},
			want: listeners(),
		},
		"insert httpproxy w/ tcpproxy w/ includes another root": {
			objs: []interface{}{proxy38, proxy39, s1},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "www.example.com", // this is proxy39, not proxy38
								ListenerName: "ingress_https",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		"insert httpproxy w/tcpproxy w/include": {
			objs: []interface{}{proxy39broot, proxy39bchild, s1},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "www.example.com",
								ListenerName: "ingress_https",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		// Issue #2218
		"insert httpproxy w/tcpproxy w/include plural": {
			objs: []interface{}{proxy39brootplural, proxy39bchild, s1},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "www.example.com",
								ListenerName: "ingress_https",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		"insert httpproxy w/ tcpproxy w/ includes valid child": {
			objs: []interface{}{proxy38, proxy40, s1},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "passthrough.example.com",
								ListenerName: "ingress_https",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		"insert httproxy w/ route w/ no services": {
			objs: []interface{}{proxy41, s1},
			want: listeners(), // expect empty, route is invalid so vhost is invalid
		},
		"insert httpproxy with pathPrefix include": {
			objs: []interface{}{
				proxy100, proxy100a, s1, s4,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/blog",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s4.Name,
											ServiceNamespace: s4.Namespace,
											ServicePort:      s4.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with pathPrefix include, child adds to pathPrefix": {
			objs: []interface{}{
				proxy100, proxy100b, s1, s4,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
								},
							),
							&Route{
								PathMatchCondition: prefixString("/blog/infotech"),
								Clusters: []*Cluster{{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s4.Name,
											ServiceNamespace: s4.Namespace,
											ServicePort:      s4.Spec.Ports[0],
										},
									},
								},
								},
							},
						),
					),
				},
			),
		},
		"insert httpproxy with pathPrefix include, child adds to pathPrefix, delegates again": {
			objs: []interface{}{
				proxy100, proxy100c, proxy100d, s1, s4, s11,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
								},
							),
							&Route{
								PathMatchCondition: prefixString("/blog/infotech"),
								Clusters: []*Cluster{{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s4.Name,
											ServiceNamespace: s4.Namespace,
											ServicePort:      s4.Spec.Ports[0],
										},
									},
								}},
							},
							routeCluster("/blog",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s4.Name,
											ServiceNamespace: s4.Namespace,
											ServicePort:      s4.Spec.Ports[0],
										},
									},
								},
							),
							&Route{
								PathMatchCondition: prefixString("/blog/it/foo"),
								Clusters: []*Cluster{{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s11.Name,
											ServiceNamespace: s11.Namespace,
											ServicePort:      s11.Spec.Ports[0],
										},
									},
								}},
							},
						),
					),
				},
			),
		},
		"insert httpproxy with no namespace for include": {
			objs: []interface{}{
				proxy101, proxy101a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include, no prefix condition on included proxy": {
			objs: []interface{}{
				proxy104, proxy104a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include, / on included proxy": {
			objs: []interface{}{
				proxy105, proxy105a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include, full prefix on included proxy": {
			objs: []interface{}{
				proxy107, proxy107a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder/withavengeance",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with include ending with /, / on included proxy": {
			objs: []interface{}{
				proxy106, proxy106a, s1, s2,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s1.Name,
											ServiceNamespace: s1.Namespace,
											ServicePort:      s1.Spec.Ports[0],
										},
									},
								},
							),
							routeCluster("/kuarder/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s2.Name,
											ServiceNamespace: s2.Namespace,
											ServicePort:      s2.Spec.Ports[0],
										},
									},
								},
							),
						),
					),
				},
			),
		},
		"insert httpproxy with multiple prefix conditions on route": {
			objs: []interface{}{
				proxy102, s1,
			},
			want: listeners(),
		},
		"insert httpproxy with multiple prefix conditions on include": {
			objs: []interface{}{
				proxy103, proxy103a, s1,
			},
			want: listeners(),
		},
		"insert httpproxy duplicate conditions on include": {
			objs: []interface{}{
				proxy108, proxy108a, proxy108b, s1, s12, s13,
			},
			want: listeners(),
		},
		"insert proxy with tcp forward without TLS termination w/ passthrough": {
			objs: []interface{}{
				proxy1a, s1,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "kuard.example.com",
								ListenerName: "ingress_https",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		// issue 1952
		"insert proxy with tcp forward without TLS termination w/ passthrough and 301 upgrade of port 80": {
			objs: []interface{}{
				proxy1d, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com",
							routeUpgrade("/", service(s1)),
						),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "kuard.example.com",
								ListenerName: "ingress_https",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s1),
								),
							},
						},
					),
				},
			),
		},
		"insert proxy with tcp forward without TLS termination w/ passthrough without 301 upgrade of port 80": {
			objs: []interface{}{
				proxy1e, s10,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("kuard.example.com",
							routeCluster("/",
								&Cluster{
									Upstream: &Service{
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s10.Name,
											ServiceNamespace: s10.Namespace,
											ServicePort:      s10.Spec.Ports[1],
										},
									},
								},
							),
						),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "kuard.example.com",
								ListenerName: "ingress_https",
							},
							TCPProxy: &TCPProxy{
								Clusters: clusters(
									service(s10),
								),
							},
							MinTLSVersion: "", // tls passthrough does not specify a TLS version; that's the domain of the backend
						},
					),
				},
			),
		},
		"insert httpproxy with route-level header manipulation": {
			objs: []interface{}{
				proxy109, s1,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com",
							routeHeaders("/", map[string]string{
								"In-Foo": "bar",
							}, []string{"In-Baz"}, map[string]string{
								"Out-Foo": "bar",
							}, []string{"Out-Baz"}, service(s1)),
						),
					),
				},
			),
		},

		// issue 1399
		"service shared across ingress and httpproxy tcpproxy": {
			objs: []interface{}{
				sec1,
				s9,
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"example.com"},
							SecretName: s1.Name,
						}},
						Rules: []networking_v1.IngressRule{{
							Host:             "example.com",
							IngressRuleValue: ingressrulev1value(backendv1(s9.Name, intstr.FromInt(80))),
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: sec1.Name,
							},
						},
						TCPProxy: &contour_api_v1.TCPProxy{
							Services: []contour_api_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
							},
							MinTLSVersion: "1.2",
							Secret:        secret(sec1),
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s9)),
							},
						},
					),
				},
			),
		},
		// issue 1954
		"httpproxy tcpproxy + permitinsecure": {
			objs: []interface{}{
				sec1,
				s9,
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: sec1.Name,
							},
						},
						Routes: []contour_api_v1.Route{{
							PermitInsecure: true,
							Services: []contour_api_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						}},
						TCPProxy: &contour_api_v1.TCPProxy{
							Services: []contour_api_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						// not upgraded because the route is permitInsecure: true
						virtualhost("example.com", prefixroute("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
							},
							MinTLSVersion: "1.2",
							Secret:        secret(sec1),
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s9)),
							},
						},
					),
				},
			),
		},
		// issue 1954
		"httpproxy tcpproxy + tlspassthrough + permitinsecure": {
			objs: []interface{}{
				s9,
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								Passthrough: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							PermitInsecure: true,
							Services: []contour_api_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						}},
						TCPProxy: &contour_api_v1.TCPProxy{
							Services: []contour_api_v1.Service{{
								Name: s9.Name,
								Port: 80,
							}},
						},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						// not upgraded because the route is permitInsecure: true
						virtualhost("example.com", prefixroute("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
							},
							MinTLSVersion: "",
							TCPProxy: &TCPProxy{
								Clusters: clusters(service(s9)),
							},
						},
					),
				},
			),
		},
		"ingressv1: Ingress then HTTPProxy with identical details, except referencing s2a": {
			objs: []interface{}{
				i17V1,
				proxy1f,
				s1,
				s2a,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", prefixroute("/", service(s2a))),
					),
				},
			),
		},
		"insert proxy with externalName service": {
			objs: []interface{}{
				proxyExternalNameService,
				s14,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: &Service{
									ExternalName: "externalservice.io",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s14.Name,
										ServiceNamespace: s14.Namespace,
										ServicePort:      s14.Spec.Ports[0],
									},
								},
								SNI: "externalservice.io",
							}},
						}),
					),
				},
			),
		},
		"insert tcp proxy with externalName service": {
			objs: []interface{}{
				tcpProxyExternalNameService,
				s14,
				sec1,
			},
			want: listeners(
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
							},
							TCPProxy: &TCPProxy{
								Clusters: []*Cluster{{
									Upstream: &Service{
										ExternalName: "externalservice.io",
										Weighted: WeightedService{
											Weight:           1,
											ServiceName:      s14.Name,
											ServiceNamespace: s14.Namespace,
											ServicePort:      s14.Spec.Ports[0],
										},
									},
									Protocol: "tls",
									SNI:      "externalservice.io",
								}},
							},
							MinTLSVersion: "1.2",
							Secret:        secret(sec1),
						},
					),
				},
			),
		},
		"insert proxy with replace header policy - route - host header": {
			objs: []interface{}{
				proxyReplaceHostHeaderRoute,
				s9,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: service(s9),
								SNI:      "bar.com",
							}},
							RequestHeadersPolicy: &HeadersPolicy{
								HostRewrite: "bar.com",
							},
						}),
					),
				},
			),
		},
		"insert proxy with replace header policy - route - host header - externalName": {
			objs: []interface{}{
				proxyReplaceHostHeaderRoute,
				s14,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: &Service{
									ExternalName: "externalservice.io",
									Weighted: WeightedService{
										Weight:           1,
										ServiceName:      s14.Name,
										ServiceNamespace: s14.Namespace,
										ServicePort:      s14.Spec.Ports[0],
									},
								},
								SNI: "bar.com",
							}},
							RequestHeadersPolicy: &HeadersPolicy{
								HostRewrite: "bar.com",
							},
						}),
					),
				},
			),
		},
		"insert proxy with replace header policy - service - host header": {
			objs: []interface{}{
				proxyReplaceHostHeaderService,
				s9,
			},
			want: listeners(),
		},
		"insert proxy with replace header policy - service - host header - externalName": {
			objs: []interface{}{
				proxyReplaceHostHeaderService,
				s14,
			},
			want: listeners(),
		},
		"insert proxy with response header policy - route - host header": {
			objs: []interface{}{
				proxy111,
				s9,
			},
			want: listeners(),
		},
		"insert proxy with response header policy - service - host header": {
			objs: []interface{}{
				proxy112,
				s9,
			},
			want: listeners(),
		},
		"insert proxy with replace header policy - host header multiple": {
			objs: []interface{}{
				proxyReplaceHostHeaderMultiple,
				s9,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{{
								Upstream: service(s9),
								SNI:      "bar.com",
							}},
							RequestHeadersPolicy: &HeadersPolicy{
								HostRewrite: "bar.com",
								Set: map[string]string{
									"X-Header": "bar.com",
									"Y-Header": "zed.com",
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with request headers policy - not host header": {
			objs: []interface{}{
				proxyReplaceNonHostHeader,
				s9,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s9),
							RequestHeadersPolicy: &HeadersPolicy{
								Set: map[string]string{
									"X-Header": "bar.com",
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with request headers policy - empty value": {
			objs: []interface{}{
				proxyReplaceHeaderEmptyValue,
				s9,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters:           clustermap(s9),
							RequestHeadersPolicy: &HeadersPolicy{
								Set: map[string]string{
									"X-Header": "",
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with cookie load balancing strategy": {
			objs: []interface{}{
				proxyCookieLoadBalancer,
				s9,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{
								{Upstream: service(s9), LoadBalancerPolicy: "Cookie"},
							},
							RequestHashPolicies: []RequestHashPolicy{
								{
									CookieHashOptions: &CookieHashOptions{
										CookieName: "X-Contour-Session-Affinity",
										TTL:        time.Duration(0),
										Path:       "/",
									},
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with load balancer request header hash policies": {
			objs: []interface{}{
				proxyLoadBalancerHashPolicyHeader,
				s9,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{
								{Upstream: service(s9), LoadBalancerPolicy: "RequestHash"},
							},
							RequestHashPolicies: []RequestHashPolicy{
								{
									Terminal: true,
									HeaderHashOptions: &HeaderHashOptions{
										HeaderName: "X-Some-Header",
									},
								},
								{
									HeaderHashOptions: &HeaderHashOptions{
										HeaderName: "X-Some-Other-Header",
									},
								},
							},
						}),
					),
				},
			),
		},
		"insert proxy with all invalid request header hash policies": {
			objs: []interface{}{
				proxyLoadBalancerHashPolicyHeaderAllInvalid,
				s9,
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", &Route{
							PathMatchCondition: prefixString("/"),
							Clusters: []*Cluster{
								{Upstream: service(s9), LoadBalancerPolicy: "RoundRobin"},
							},
						}),
					),
				},
			),
		},
		"httpproxy with fallback certificate enabled": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []interface{}{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
								routes:       routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							Secret:              secret(sec1),
							FallbackCertificate: secret(fallbackCertificateSecret),
						},
					),
				},
			),
		},
		"httpproxy with fallback certificate enabled - cert delegation not configured": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "root",
			objs: []interface{}{
				sec4,
				s9,
				fallbackCertificateSecret,
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(),
		},
		"httpproxy with fallback certificate enabled - cert delegation configured all namespaces": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "root",
			objs: []interface{}{
				sec1,
				s9,
				fallbackCertificateSecretRootNamespace,
				&contour_api_v1.TLSCertificateDelegation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbackcertdelegation",
						Namespace: "root",
					},
					Spec: contour_api_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_api_v1.CertificateDelegation{{
							SecretName:       "fallbacksecret",
							TargetNamespaces: []string{"*"},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
								routes:       routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							Secret:              secret(sec1),
							FallbackCertificate: secret(fallbackCertificateSecretRootNamespace),
						},
					),
				},
			),
		},
		"httpproxy with fallback certificate enabled - cert delegation configured single namespaces": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "root",
			objs: []interface{}{
				sec1,
				s9,
				fallbackCertificateSecretRootNamespace,
				&contour_api_v1.TLSCertificateDelegation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fallbackcertdelegation",
						Namespace: "root",
					},
					Spec: contour_api_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_api_v1.CertificateDelegation{{
							SecretName:       "fallbacksecret",
							TargetNamespaces: []string{"default"},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
								routes:       routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							Secret:              secret(sec1),
							FallbackCertificate: secret(fallbackCertificateSecretRootNamespace),
						},
					),
				},
			),
		},
		"httpproxy with fallback certificate enabled - no tls secret": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []interface{}{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: nil,
		},
		"httpproxy with fallback certificate enabled along with ClientValidation": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []interface{}{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								EnableFallbackCertificate: true,
								ClientValidation: &contour_api_v1.DownstreamValidation{
									CACertificate: cert1.Name,
								},
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: nil,
		},
		"httpproxy with fallback certificate enabled - another not enabled": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []interface{}{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: true,
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx-disabled",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "projectcontour.io",
							TLS: &contour_api_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: false,
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
						virtualhost("projectcontour.io", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
								routes:       routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							Secret:              secret(sec1),
							FallbackCertificate: secret(fallbackCertificateSecret),
						},
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "projectcontour.io",
								ListenerName: "ingress_https",
								routes:       routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							Secret:              secret(sec1),
							FallbackCertificate: nil,
						},
					),
				},
			),
		},
		"httpproxy with fallback certificate enabled - bad fallback cert": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "badnamespaces",
			objs: []interface{}{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: sec1.Name,
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
								routes:       routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							Secret:              secret(sec1),
							FallbackCertificate: nil,
						},
					),
				},
			),
		},
		"httpproxy with fallback certificate disabled - fallback cert specified": {
			fallbackCertificateName:      "fallbacksecret",
			fallbackCertificateNamespace: "default",
			objs: []interface{}{
				sec1,
				s9,
				fallbackCertificateSecret,
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
							TLS: &contour_api_v1.TLS{
								SecretName:                sec1.Name,
								EnableFallbackCertificate: false,
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "nginx",
								Port: 80,
							}},
						}},
					},
				},
			},
			want: listeners(
				&Listener{
					Port: 80,
					VirtualHosts: virtualhosts(
						virtualhost("example.com", routeUpgrade("/", service(s9))),
					),
				},
				&Listener{
					Port: 443,
					VirtualHosts: virtualhosts(
						&SecureVirtualHost{
							VirtualHost: VirtualHost{
								Name:         "example.com",
								ListenerName: "ingress_https",
								routes:       routes(routeUpgrade("/", service(s9))),
							},
							MinTLSVersion:       "1.2",
							Secret:              secret(sec1),
							FallbackCertificate: nil,
						},
					),
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					FieldLogger: fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{
						DisablePermitInsecure: tc.disablePermitInsecure,
						FallbackCertificate: &types.NamespacedName{
							Name:      tc.fallbackCertificateName,
							Namespace: tc.fallbackCertificateNamespace,
						},
					},
					&ListenerProcessor{},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			got := make(map[int]*Listener)
			dag.Visit(listenerMap(got).Visit)

			want := make(map[int]*Listener)
			for _, v := range tc.want {
				if l, ok := v.(*Listener); ok {
					want[l.Port] = l
				}
			}
			assert.Equal(t, want, got)
		})
	}
}

type listenerMap map[int]*Listener

func (lm listenerMap) Visit(v Vertex) {
	if l, ok := v.(*Listener); ok {
		lm[l.Port] = l
	}
}

func backendv1(name string, port intstr.IntOrString) *networking_v1.IngressBackend {

	var v1port networking_v1.ServiceBackendPort

	switch port.Type {
	case intstr.Int:
		v1port = networking_v1.ServiceBackendPort{
			Number: port.IntVal,
		}
	case intstr.String:
		v1port = networking_v1.ServiceBackendPort{
			Name: port.StrVal,
		}
	}

	return &networking_v1.IngressBackend{
		Service: &networking_v1.IngressServiceBackend{
			Name: name,
			Port: v1port,
		},
	}
}

func ingressrulev1value(backend *networking_v1.IngressBackend) networking_v1.IngressRuleValue {
	return networking_v1.IngressRuleValue{
		HTTP: &networking_v1.HTTPIngressRuleValue{
			Paths: []networking_v1.HTTPIngressPath{{
				Backend: *backend,
			}},
		},
	}
}

func TestDAGRootNamespaces(t *testing.T) {
	proxy1 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "allowed1",
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

	// proxy2 is like proxy1, but in a different namespace
	proxy2 := &contour_api_v1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "allowed2",
		},
		Spec: contour_api_v1.HTTPProxySpec{
			VirtualHost: &contour_api_v1.VirtualHost{
				Fqdn: "example2.com",
			},
			Routes: []contour_api_v1.Route{{
				Services: []contour_api_v1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "allowed1",
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
			Name:      "kuard",
			Namespace: "allowed2",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}

	tests := map[string]struct {
		rootNamespaces []string
		objs           []interface{}
		want           int
	}{
		"nil root httpproxy namespaces": {
			objs: []interface{}{proxy1, s2},
			want: 1,
		},
		"empty root httpproxy namespaces": {
			objs: []interface{}{proxy1, s2},
			want: 1,
		},
		"single root namespace with root httpproxy": {
			rootNamespaces: []string{"allowed1"},
			objs:           []interface{}{proxy1, s2},
			want:           1,
		},
		"multiple root namespaces, one with a root httpproxy": {
			rootNamespaces: []string{"foo", "allowed1", "bar"},
			objs:           []interface{}{proxy1, s2},
			want:           1,
		},
		"multiple root namespaces, each with a root httpproxy": {
			rootNamespaces: []string{"foo", "allowed1", "allowed2"},
			objs:           []interface{}{proxy1, proxy2, s2, s3},
			want:           2,
		},
		"root httpproxy defined outside single root namespaces": {
			rootNamespaces: []string{"foo"},
			objs:           []interface{}{proxy1},
			want:           0,
		},
		"root httpproxy defined outside multiple root namespaces": {
			rootNamespaces: []string{"foo", "bar"},
			objs:           []interface{}{proxy1},
			want:           0,
		},
		"two root httpproxy, one inside root namespace, one outside": {
			rootNamespaces: []string{"foo", "allowed2"},
			objs:           []interface{}{proxy1, proxy2, s3},
			want:           1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := Builder{
				Source: KubernetesCache{
					RootNamespaces: tc.rootNamespaces,
					FieldLogger:    fixture.NewTestLogger(t),
				},
				Processors: []Processor{
					&IngressProcessor{
						FieldLogger: fixture.NewTestLogger(t),
					},
					&HTTPProxyProcessor{},
					&ListenerProcessor{},
				},
			}

			for _, o := range tc.objs {
				builder.Source.Insert(o)
			}
			dag := builder.Build()

			var count int
			dag.Visit(func(v Vertex) {
				v.Visit(func(v Vertex) {
					if _, ok := v.(*VirtualHost); ok {
						count++
					}
				})
			})

			if tc.want != count {
				t.Errorf("wanted %d vertices, but got %d", tc.want, count)
			}
		})
	}
}

func TestHttpPaths(t *testing.T) {
	tests := map[string]struct {
		rule networking_v1.IngressRule
		want []networking_v1.HTTPIngressPath
	}{
		"zero value": {
			rule: networking_v1.IngressRule{},
			want: nil,
		},
		"empty paths": {
			rule: networking_v1.IngressRule{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{},
				},
			},
			want: nil,
		},
		"several paths": {
			rule: networking_v1.IngressRule{
				IngressRuleValue: networking_v1.IngressRuleValue{
					HTTP: &networking_v1.HTTPIngressRuleValue{
						Paths: []networking_v1.HTTPIngressPath{{
							Backend: *backendv1("kuard", intstr.FromString("http")),
						}, {
							Path:    "/kuarder",
							Backend: *backendv1("kuarder", intstr.FromInt(8080)),
						}},
					},
				},
			},
			want: []networking_v1.HTTPIngressPath{{
				Backend: *backendv1("kuard", intstr.FromString("http")),
			}, {
				Path:    "/kuarder",
				Backend: *backendv1("kuarder", intstr.FromInt(8080)),
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := httppaths(tc.rule)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDetermineSNI(t *testing.T) {
	tests := map[string]struct {
		routeRequestHeaders   *HeadersPolicy
		clusterRequestHeaders *HeadersPolicy
		service               *Service
		want                  string
	}{
		"default SNI": {
			routeRequestHeaders:   nil,
			clusterRequestHeaders: nil,
			service:               &Service{},
			want:                  "",
		},
		"route request headers set": {
			routeRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			clusterRequestHeaders: nil,
			service:               &Service{},
			want:                  "containersteve.com",
		},
		"service request headers set": {
			routeRequestHeaders: nil,
			clusterRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			service: &Service{},
			want:    "containersteve.com",
		},
		"service request headers set overrides route": {
			routeRequestHeaders: &HeadersPolicy{
				HostRewrite: "incorrect.com",
			},
			clusterRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			service: &Service{},
			want:    "containersteve.com",
		},
		"route request headers override externalName": {
			routeRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			clusterRequestHeaders: nil,
			service: &Service{
				ExternalName: "externalname.com",
			},
			want: "containersteve.com",
		},
		"service request headers override externalName": {
			routeRequestHeaders: nil,
			clusterRequestHeaders: &HeadersPolicy{
				HostRewrite: "containersteve.com",
			},
			service: &Service{
				ExternalName: "externalname.com",
			},
			want: "containersteve.com",
		},
		"only externalName set": {
			routeRequestHeaders:   nil,
			clusterRequestHeaders: nil,
			service: &Service{
				ExternalName: "externalname.com",
			},
			want: "externalname.com",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := determineSNI(tc.routeRequestHeaders, tc.clusterRequestHeaders, tc.service)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEnforceRoute(t *testing.T) {
	tests := map[string]struct {
		tlsEnabled     bool
		permitInsecure bool
		want           bool
	}{
		"tls not enabled": {
			tlsEnabled:     false,
			permitInsecure: false,
			want:           false,
		},
		"tls enabled": {
			tlsEnabled:     true,
			permitInsecure: false,
			want:           true,
		},
		"tls enabled but insecure requested": {
			tlsEnabled:     true,
			permitInsecure: true,
			want:           false,
		},
		"tls not enabled but insecure requested": {
			tlsEnabled:     false,
			permitInsecure: true,
			want:           false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := routeEnforceTLS(tc.tlsEnabled, tc.permitInsecure)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestValidateHeaderAlteration(t *testing.T) {
	tests := []struct {
		name    string
		in      *contour_api_v1.HeadersPolicy
		dyn     map[string]string
		dhp     *HeadersPolicy
		want    *HeadersPolicy
		wantErr error
	}{{
		name: "empty is fine",
	}, {
		name: "set two, remove one",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "bar",
			}, {
				Name:  "k-baz", // This gets canonicalized
				Value: "blah",
			}},
			Remove: []string{"K-Nada"},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: nil,
		want: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo": "bar",
				"K-Baz": "blah",
			},
			Remove: []string{"K-Nada"},
		},
	}, {
		name: "duplicate set",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "bar",
			}, {
				Name:  "k-foo", // This gets canonicalized
				Value: "blah",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`duplicate header addition: "K-Foo"`),
	}, {
		name: "duplicate remove",
		in: &contour_api_v1.HeadersPolicy{
			Remove: []string{"K-Foo", "k-foo"},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`duplicate header removal: "K-Foo"`),
	}, {
		name: "invalid set header",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "  K-Foo",
				Value: "bar",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`invalid set header "  K-Foo": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')]`),
	}, {
		name: "invalid set default header",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Set: map[string]string{
				"  K-Foo": "bar",
			},
		},
		wantErr: errors.New(`invalid set header "  K-Foo": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')]`),
	}, {
		name: "invalid remove header",
		in: &contour_api_v1.HeadersPolicy{
			Remove: []string{"  K-Foo"},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`invalid remove header "  K-Foo": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')]`),
	}, {
		name: "invalid remove default header",
		in: &contour_api_v1.HeadersPolicy{
			Remove: []string{"  K-Foo"},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Remove: []string{"  K-Foo"},
		},
		wantErr: errors.New(`invalid remove header "  K-Foo": [a valid HTTP header must consist of alphanumeric characters or '-' (e.g. 'X-Header-Name', regex used for validation is '[-A-Za-z0-9]+')]`),
	}, {
		name: "invalid set header (special headers)",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "Host",
				Value: "bar",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp:     nil,
		wantErr: errors.New(`rewriting "Host" header is not supported`),
	}, {
		name: "invalid set default header (special headers)",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "ook?",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Set: map[string]string{
				"Host": "bar",
			},
		},
		wantErr: errors.New(`rewriting "Host" header is not supported`),
	}, {
		name: "percents are escaped",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "100%",
			}, {
				Name:  "Lot-Of-Percents",
				Value: "%%%%%",
			}, {
				Name:  "k-baz",                      // This gets canonicalized
				Value: "%DOWNSTREAM_LOCAL_ADDRESS%", // This is a known Envoy dynamic header
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: nil,
		want: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo":           "100%%",
				"K-Baz":           "%DOWNSTREAM_LOCAL_ADDRESS%",
				"Lot-Of-Percents": "%%%%%%%%%%",
			},
		},
	}, {
		name: "dynamic service headers",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "l5d-dst-override",
				Value: "%CONTOUR_SERVICE_NAME%.%CONTOUR_NAMESPACE%.svc.cluster.local:%CONTOUR_SERVICE_PORT%",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE":    "myns",
			"CONTOUR_SERVICE_NAME": "myservice",
			"CONTOUR_SERVICE_PORT": "80",
		},
		dhp: nil,
		want: &HeadersPolicy{
			Set: map[string]string{
				"L5d-Dst-Override": "myservice.myns.svc.cluster.local:80",
			},
		},
	}, {
		name: "dynamic service headers without service name and port",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "l5d-dst-override",
				Value: "%CONTOUR_SERVICE_NAME%.%CONTOUR_NAMESPACE%.svc.cluster.local:%CONTOUR_SERVICE_PORT%",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: nil,
		want: &HeadersPolicy{
			Set: map[string]string{
				"L5d-Dst-Override": "%%CONTOUR_SERVICE_NAME%%.myns.svc.cluster.local:%%CONTOUR_SERVICE_PORT%%",
			},
		},
	}, {
		name: "default headers are combined with given headers and escaped",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "100%",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Set: map[string]string{
				"k-baz":           "%DOWNSTREAM_LOCAL_ADDRESS%", // This gets canonicalized
				"Lot-Of-Percents": "%%%%%",
			},
		},
		want: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo":           "100%%",
				"K-Baz":           "%DOWNSTREAM_LOCAL_ADDRESS%",
				"Lot-Of-Percents": "%%%%%%%%%%",
			},
		},
	}, {
		name: "default headers do not replace given headers",
		in: &contour_api_v1.HeadersPolicy{
			Set: []contour_api_v1.HeaderValue{{
				Name:  "K-Foo",
				Value: "100%",
			}},
		},
		dyn: map[string]string{
			"CONTOUR_NAMESPACE": "myns",
		},
		dhp: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo": "50%",
			},
		},
		want: &HeadersPolicy{
			Set: map[string]string{
				"K-Foo": "100%%",
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, gotErr := headersPolicyService(test.dhp, test.in, test.dyn)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.wantErr, gotErr)
		})
	}
}

func TestBuilderRunsProcessorsInOrder(t *testing.T) {
	var got []string

	b := Builder{
		Processors: []Processor{
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "foo") }),
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "bar") }),
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "baz") }),
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "abc") }),
			ProcessorFunc(func(*DAG, *KubernetesCache) { got = append(got, "def") }),
		},
	}

	b.Build()

	assert.Equal(t, []string{"foo", "bar", "baz", "abc", "def"}, got)
}

func routes(routes ...*Route) map[string]*Route {
	if len(routes) == 0 {
		return nil
	}
	m := make(map[string]*Route)
	for _, r := range routes {
		m[conditionsToString(r)] = r
	}
	return m
}

func directResponseRoute(prefix string, statusCode uint32) *Route {
	return &Route{
		PathMatchCondition: prefixString(prefix),
		DirectResponse:     &DirectResponse{StatusCode: statusCode},
	}
}

func directResponseRouteService(prefix string, statusCode uint32, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: prefixString(prefix),
		DirectResponse:     &DirectResponse{StatusCode: statusCode},
		Clusters:           clustersWeight(services...),
	}
}

func httpRouteMatch(pathType gatewayapi_v1alpha1.PathMatchType, value string) []gatewayapi_v1alpha1.HTTPRouteMatch {
	return []gatewayapi_v1alpha1.HTTPRouteMatch{{
		Path: &gatewayapi_v1alpha1.HTTPPathMatch{
			Type:  pathMatchTypePtr(pathType),
			Value: pointer.StringPtr(value),
		},
	}}
}

func httpRouteForwards(forwards ...[]gatewayapi_v1alpha1.HTTPRouteForwardTo) []gatewayapi_v1alpha1.HTTPRouteForwardTo {
	var fwds []gatewayapi_v1alpha1.HTTPRouteForwardTo

	for _, f := range forwards {
		fwds = append(fwds, f...)
	}
	return fwds
}

func httpRouteForwardTo(serviceName string, port int, weight int32) []gatewayapi_v1alpha1.HTTPRouteForwardTo {
	return []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
		ServiceName: pointer.StringPtr(serviceName),
		Port:        gatewayPort(port),
		Weight:      pointer.Int32Ptr(weight),
	}}
}

func prefixroute(prefix string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: prefixString(prefix),
		Clusters:           clusters(services...),
	}
}

func prefixrouteHTTPRoute(prefix string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: prefixString(prefix),
		Clusters:           clustersWeight(services...),
	}
}

func exactrouteHTTPRoute(path string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)
	return &Route{
		PathMatchCondition: &ExactMatchCondition{Path: path},
		Clusters:           clustersWeight(services...),
	}
}

func routeProtocol(prefix string, protocol string, first *Service, rest ...*Service) *Route {
	services := append([]*Service{first}, rest...)

	cs := clusters(services...)
	for _, c := range cs {
		c.Protocol = protocol
	}
	return &Route{
		PathMatchCondition: prefixString(prefix),
		Clusters:           cs,
	}
}

func routeCluster(prefix string, first *Cluster, rest ...*Cluster) *Route {
	return &Route{
		PathMatchCondition: prefixString(prefix),
		Clusters:           append([]*Cluster{first}, rest...),
	}
}

func routeUpgrade(prefix string, first *Service, rest ...*Service) *Route {
	r := prefixroute(prefix, first, rest...)
	r.HTTPSUpgrade = true
	return r
}

func routeWebsocket(prefix string, first *Service, rest ...*Service) *Route {
	r := prefixroute(prefix, first, rest...)
	r.Websocket = true
	return r
}

func routeHeaders(prefix string, requestSet map[string]string, requestRemove []string, responseSet map[string]string, responseRemove []string, first *Service, rest ...*Service) *Route {
	r := prefixroute(prefix, first, rest...)
	r.RequestHeadersPolicy = &HeadersPolicy{
		Set:    requestSet,
		Remove: requestRemove,
	}
	r.ResponseHeadersPolicy = &HeadersPolicy{
		Set:    responseSet,
		Remove: responseRemove,
	}
	return r
}

func clusterHeaders(requestSet map[string]string, requestAdd map[string]string, requestRemove []string, hostRewrite string, services ...*Service) (c []*Cluster) {
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: s,
			Protocol: s.Protocol,
			RequestHeadersPolicy: &HeadersPolicy{
				Set:         requestSet,
				Add:         requestAdd,
				Remove:      requestRemove,
				HostRewrite: hostRewrite,
			},
			Weight: s.Weighted.Weight,
		})
	}
	return c
}

func clusters(services ...*Service) (c []*Cluster) {
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: s,
			Protocol: s.Protocol,
		})
	}
	return c
}

func clustersWeight(services ...*Service) (c []*Cluster) {
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: s,
			Protocol: s.Protocol,
			Weight:   s.Weighted.Weight,
		})
	}
	return c
}

func service(s *v1.Service) *Service {
	return &Service{
		Weighted: WeightedService{
			Weight:           1,
			ServiceName:      s.Name,
			ServiceNamespace: s.Namespace,
			ServicePort:      s.Spec.Ports[0],
		},
	}
}

func clustermap(services ...*v1.Service) []*Cluster {
	var c []*Cluster
	for _, s := range services {
		c = append(c, &Cluster{
			Upstream: service(s),
		})
	}
	return c
}

func secret(s *v1.Secret) *Secret {
	return &Secret{
		Object: s,
	}
}

func virtualhosts(vx ...Vertex) []Vertex {
	return vx
}

func virtualhost(name string, first *Route, rest ...*Route) *VirtualHost {
	return &VirtualHost{
		Name:         name,
		ListenerName: "ingress_http",
		routes:       routes(append([]*Route{first}, rest...)...),
	}
}

func securevirtualhost(name string, sec *v1.Secret, first *Route, rest ...*Route) *SecureVirtualHost {
	return &SecureVirtualHost{
		VirtualHost: VirtualHost{
			Name:         name,
			ListenerName: "ingress_https",
			routes:       routes(append([]*Route{first}, rest...)...),
		},
		MinTLSVersion: "1.2",
		Secret:        secret(sec),
	}
}

func listeners(ls ...*Listener) []Vertex {
	var v []Vertex
	for _, l := range ls {
		v = append(v, l)
	}
	return v
}

func prefixString(prefix string) MatchCondition {
	return &PrefixMatchCondition{Prefix: prefix, PrefixMatchType: PrefixMatchString}
}
func prefixSegment(prefix string) MatchCondition {
	return &PrefixMatchCondition{Prefix: prefix, PrefixMatchType: PrefixMatchSegment}
}
func exact(path string) MatchCondition  { return &ExactMatchCondition{Path: path} }
func regex(regex string) MatchCondition { return &RegexMatchCondition{Regex: regex} }

func withMirror(r *Route, mirror *Service) *Route {
	r.MirrorPolicy = &MirrorPolicy{
		Cluster: &Cluster{
			Upstream: mirror,
		},
	}
	return r
}

func routeSelectTypePtr(rst gatewayapi_v1alpha1.RouteSelectType) *gatewayapi_v1alpha1.RouteSelectType {
	return &rst
}
