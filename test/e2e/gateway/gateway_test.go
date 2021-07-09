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

// +build e2e

package gateway

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var f = e2e.NewFramework(false)

func TestGatewayAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway API tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Deployment.EnsureResourcesForLocalContour())
})

var _ = AfterSuite(func() {
	f.DeleteNamespace(f.Deployment.Namespace.Name, true)
	gexec.CleanupBuildArtifacts()
})

var _ = Describe("Gateway API", func() {
	var (
		contourCmd            *gexec.Session
		contourConfig         *config.Parameters
		contourConfigFile     string
		additionalContourArgs []string

		contourGatewayClass *gatewayv1alpha1.GatewayClass
		contourGateway      *gatewayv1alpha1.Gateway
	)

	// Creates specified gateway in namespace and runs namespaced test
	// body. Modifies contour config to point to gateway.
	testWithGateway := func(gateway *gatewayv1alpha1.Gateway, gatewayClass *gatewayv1alpha1.GatewayClass, body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
		return func(namespace string) {

			Context(fmt.Sprintf("with gateway %s/%s, controllerName: %s", namespace, gateway.Name, gatewayClass.Spec.Controller), func() {
				BeforeEach(func() {
					// Ensure gateway created in this test's namespace.
					gateway.Namespace = namespace
					// Update contour config to point to specified gateway.
					contourConfig.GatewayConfig = &config.GatewayParameters{
						ControllerName: gatewayClass.Spec.Controller,
					}

					contourGatewayClass = gatewayClass
					contourGateway = gateway
				})
				AfterEach(func() {
					require.NoError(f.T(), f.DeleteGateway(gateway, false))
				})

				body(namespace)
			})
		}
	}

	BeforeEach(func() {
		// Contour config file contents, can be modified in nested
		// BeforeEach.
		contourConfig = &config.Parameters{}

		// Default contour serve command line arguments can be appended to in
		// nested BeforeEach.
		additionalContourArgs = []string{}
	})

	// JustBeforeEach is called after each of the nested BeforeEach are
	// called, so it is a final setup step before running a test.
	// A nested BeforeEach may have modified Contour config, so we wait
	// until here to start Contour.
	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Wait for Envoy to be healthy.
		require.NoError(f.T(), f.Deployment.WaitForEnvoyDaemonSetUpdated())

		f.CreateGatewayClassAndWaitFor(contourGatewayClass, gatewayClassValid)
		f.CreateGatewayAndWaitFor(contourGateway, gatewayValid)
	})

	AfterEach(func() {
		require.NoError(f.T(), f.DeleteGatewayClass(contourGatewayClass, false))
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	Describe("HTTPRoute: Insecure (Non-TLS) Gateway", func() {
		testWithHTTPGateway := func(body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
			gatewayClass := getGatewayClass()
			gw := &gatewayv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "http",
				},
				Spec: gatewayv1alpha1.GatewaySpec{
					GatewayClassName: gatewayClass.Name,
					Listeners: []gatewayv1alpha1.Listener{
						{
							Protocol: gatewayv1alpha1.HTTPProtocolType,
							Port:     gatewayv1alpha1.PortNumber(80),
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "HTTPRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": "filter"},
								},
							},
						},
					},
				},
			}

			return testWithGateway(gw, gatewayClass, body)
		}

		f.NamespacedTest("001-path-condition-match", testWithHTTPGateway(testGatewayPathConditionMatch))

		f.NamespacedTest("002-header-condition-match", testWithHTTPGateway(testGatewayHeaderConditionMatch))

		f.NamespacedTest("003-invalid-forward-to", testWithHTTPGateway(testInvalidForwardTo))

		f.NamespacedTest("005-request-header-modifier-forward-to", testWithHTTPGateway(testRequestHeaderModifierForwardTo))

		f.NamespacedTest("005-request-header-modifier-rule", testWithHTTPGateway(testRequestHeaderModifierRule))

		f.NamespacedTest("006-host-rewrite", testWithHTTPGateway(testHostRewrite))

		f.NamespacedTest("007-gateway-allow-type", testWithHTTPGateway(testGatewayAllowType))
	})

	Describe("HTTPRoute: TLS Gateway", func() {
		testWithHTTPSGateway := func(hostname string, body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
			gatewayClass := getGatewayClass()
			gw := &gatewayv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "https",
				},
				Spec: gatewayv1alpha1.GatewaySpec{
					GatewayClassName: gatewayClass.Name,
					Listeners: []gatewayv1alpha1.Listener{
						{
							Protocol: gatewayv1alpha1.HTTPProtocolType,
							Port:     gatewayv1alpha1.PortNumber(80),
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "HTTPRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"type": "insecure"},
								},
							},
						},
						{
							Protocol: gatewayv1alpha1.HTTPSProtocolType,
							Port:     gatewayv1alpha1.PortNumber(443),
							TLS: &gatewayv1alpha1.GatewayTLSConfig{
								CertificateRef: &gatewayv1alpha1.LocalObjectReference{
									Group: "core",
									Kind:  "Secret",
									Name:  "tlscert",
								},
							},
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "HTTPRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"type": "secure"},
								},
							},
						},
					},
				},
			}
			return testWithGateway(gw, gatewayClass, func(namespace string) {
				Context(fmt.Sprintf("with TLS secret %s/tlscert for hostname %s", namespace, hostname), func() {
					BeforeEach(func() {
						f.Certs.CreateSelfSignedCert(namespace, "tlscert", "tlscert", hostname)
					})

					body(namespace)
				})
			})
		}

		f.NamespacedTest("004-httproute-tls-gateway", testWithHTTPSGateway("tls-gateway.projectcontour.io", testTLSGateway))

		f.NamespacedTest("009-tls-wildcard-host", testWithHTTPSGateway("*.wildcardhost.gateway.projectcontour.io", testTLSWildcardHost))
	})

	Describe("TLSRoute: Gateway", func() {
		gatewayClass := getGatewayClass()
		gw := &gatewayv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name: "tls",
			},
			Spec: gatewayv1alpha1.GatewaySpec{
				GatewayClassName: gatewayClass.Name,
				Listeners: []gatewayv1alpha1.Listener{{
					Protocol: gatewayv1alpha1.TLSProtocolType,
					TLS: &gatewayv1alpha1.GatewayTLSConfig{
						Mode: tlsModeTypePtr(gatewayv1alpha1.TLSModePassthrough),
					},
					Port: gatewayv1alpha1.PortNumber(443),
					Routes: gatewayv1alpha1.RouteBindingSelector{
						Kind: "TLSRoute",
						Namespaces: &gatewayv1alpha1.RouteNamespaces{
							From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
						},
					},
				}},
			},
		}
		f.NamespacedTest("008-tlsroute", testWithGateway(gw, gatewayClass, testTLSRoutePassthrough))
	})

	Describe("TLSRoute Gateway: Mode: Passthrough", func() {
		gatewayClass := getGatewayClass()
		gw := &gatewayv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name: "tls-passthrough",
			},
			Spec: gatewayv1alpha1.GatewaySpec{
				GatewayClassName: gatewayClass.Name,
				Listeners: []gatewayv1alpha1.Listener{
					{
						Protocol: gatewayv1alpha1.TLSProtocolType,
						Port:     gatewayv1alpha1.PortNumber(443),
						TLS: &gatewayv1alpha1.GatewayTLSConfig{
							Mode: tlsModeTypePtr(gatewayv1alpha1.TLSModePassthrough),
						},
						Routes: gatewayv1alpha1.RouteBindingSelector{
							Kind: "TLSRoute",
							Namespaces: &gatewayv1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
							},
						},
					},
				},
			},
		}
		f.NamespacedTest("008-tlsroute-mode-passthrough", testWithGateway(gw, gatewayClass, testTLSRoutePassthrough))
	})

	Describe("TLSRoute Gateway: Mode: Terminate", func() {

		testWithTLSGateway := func(hostname string, gatewayClass *gatewayv1alpha1.GatewayClass, body e2e.NamespacedTestBody) e2e.NamespacedTestBody {
			gw := &gatewayv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tls-passthrough",
				},
				Spec: gatewayv1alpha1.GatewaySpec{
					GatewayClassName: gatewayClass.Name,
					Listeners: []gatewayv1alpha1.Listener{
						{
							Protocol: gatewayv1alpha1.TLSProtocolType,
							Port:     gatewayv1alpha1.PortNumber(443),
							TLS: &gatewayv1alpha1.GatewayTLSConfig{
								Mode: tlsModeTypePtr(gatewayv1alpha1.TLSModeTerminate),
								CertificateRef: &gatewayv1alpha1.LocalObjectReference{
									Group: "core",
									Kind:  "Secret",
									Name:  "tlscert",
								},
							},
							Routes: gatewayv1alpha1.RouteBindingSelector{
								Kind: "TLSRoute",
								Namespaces: &gatewayv1alpha1.RouteNamespaces{
									From: routeSelectTypePtr(gatewayv1alpha1.RouteSelectAll),
								},
							},
						},
					},
				},
			}
			return testWithGateway(gw, gatewayClass, func(namespace string) {
				Context(fmt.Sprintf("with TLS secret %s/tlscert for hostname %s", namespace, hostname), func() {
					BeforeEach(func() {
						f.Certs.CreateSelfSignedCert(namespace, "tlscert", "tlscert", hostname)
					})

					body(namespace)
				})
			})
		}

		gatewayClass := getGatewayClass()

		f.NamespacedTest("008-tlsroute-mode-terminate", testWithTLSGateway("tlsroute.gatewayapi.projectcontour.io", gatewayClass, testTLSRouteTerminate))
	})
})

func stringPtr(s string) *string {
	return &s
}

func portNumPtr(port int) *gatewayv1alpha1.PortNumber {
	pn := gatewayv1alpha1.PortNumber(port)
	return &pn
}

func routeSelectTypePtr(val gatewayv1alpha1.RouteSelectType) *gatewayv1alpha1.RouteSelectType {
	return &val
}

func pathMatchTypePtr(val gatewayv1alpha1.PathMatchType) *gatewayv1alpha1.PathMatchType {
	return &val
}

func headerMatchTypePtr(val gatewayv1alpha1.HeaderMatchType) *gatewayv1alpha1.HeaderMatchType {
	return &val
}

func gatewayAllowTypePtr(val gatewayv1alpha1.GatewayAllowType) *gatewayv1alpha1.GatewayAllowType {
	return &val
}

func tlsModeTypePtr(mode gatewayv1alpha1.TLSModeType) *gatewayv1alpha1.TLSModeType {
	return &mode
}

// httpRouteAdmitted returns true if the route has a .status.conditions
// entry of "Admitted: true".
func httpRouteAdmitted(route *gatewayv1alpha1.HTTPRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Gateways {
		for _, cond := range gw.Conditions {
			if cond.Type == string(gatewayv1alpha1.ConditionRouteAdmitted) && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
	}

	return false
}

// tlsRouteAdmitted returns true if the route has a .status.conditions
// entry of "Admitted: true".
func tlsRouteAdmitted(route *gatewayv1alpha1.TLSRoute) bool {
	if route == nil {
		return false
	}

	for _, gw := range route.Status.Gateways {
		for _, cond := range gw.Conditions {
			if cond.Type == string(gatewayv1alpha1.ConditionRouteAdmitted) && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
	}

	return false
}

// gatewayValid returns true if the gateway has a .status.conditions
// entry of Ready: true".
func gatewayValid(gateway *gatewayv1alpha1.Gateway) bool {
	if gateway == nil {
		return false
	}

	for _, cond := range gateway.Status.Conditions {
		if cond.Type == string(gatewayv1alpha1.GatewayConditionReady) && cond.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

// gatewayClassValid returns true if the gateway has a .status.conditions
// entry of Admitted: true".
func gatewayClassValid(gatewayClass *gatewayv1alpha1.GatewayClass) bool {
	if gatewayClass == nil {
		return false
	}

	for _, cond := range gatewayClass.Status.Conditions {
		if cond.Type == string(gatewayv1alpha1.GatewayClassConditionStatusAdmitted) && cond.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

func getRandomNumber() int64 {
	nBig, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		panic(err)
	}
	return nBig.Int64()
}

func getGatewayClass() *gatewayv1alpha1.GatewayClass {
	randNumber := getRandomNumber()

	return &gatewayv1alpha1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("contour-class-%d", randNumber),
		},
		Spec: gatewayv1alpha1.GatewayClassSpec{
			Controller: fmt.Sprintf("projectcontour.io/ingress-controller-%d", randNumber),
		},
	}
}
