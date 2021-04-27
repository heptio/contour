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
	"testing"

	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testInvalidForwardTo(t *testing.T, fx *e2e.Framework) {
	namespace := "gateway-003-invalid-forward-to"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.Echo.Deploy(namespace, "echo-slash-default")

	route := &gatewayv1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "http-filter-1",
			Labels:    map[string]string{"app": "filter"},
		},
		Spec: gatewayv1alpha1.HTTPRouteSpec{
			Hostnames: []gatewayv1alpha1.Hostname{"invalidforwardto.projectcontour.io"},
			Gateways: &gatewayv1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowAll),
			},
			Rules: []gatewayv1alpha1.HTTPRouteRule{
				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: &gatewayv1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
								Value: stringPtr("/invalidref"),
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("invalid"),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: &gatewayv1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
								Value: stringPtr("/invalidport"),
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-default"),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: &gatewayv1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
								Value: stringPtr("/invalidservicename"),
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr(""),
							Port:        portNumPtr(80),
						},
					},
				},

				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: &gatewayv1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
								Value: stringPtr("/"),
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-slash-default"),
							Port:        portNumPtr(80),
						},
					},
				},
			},
		},
	}
	// The HTTPRoute won't be fully valid, so just wait for it to have any Gateway
	// status.
	fx.CreateHTTPRouteAndWaitFor(route, func(route *gatewayv1alpha1.HTTPRoute) bool {
		return len(route.Status.Gateways) > 0
	})

	type scenario struct {
		path           string
		expectResponse int
		expectService  string
	}

	cases := []scenario{
		{
			path:           "/",
			expectResponse: 200,
			expectService:  "echo-slash-default",
		},
		{
			path:           "/invalidref",
			expectResponse: 503,
		},
		{
			path:           "/invalidport",
			expectResponse: 503,
		},
		{
			path:           "/invalidservicename",
			expectResponse: 503,
		},
	}

	for _, tc := range cases {
		res, ok := fx.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      string(route.Spec.Hostnames[0]),
			Path:      tc.path,
			Condition: e2e.HasStatusCode(tc.expectResponse),
		})
		if !assert.Truef(t, ok, "expected %d response code, got %d", tc.expectResponse, res.StatusCode) {
			continue
		}
		if res.StatusCode != 200 {
			// If we expected something other than a 200,
			// then we don't need to check the body.
			continue
		}

		body := fx.GetEchoResponseBody(res.Body)
		assert.Equal(t, namespace, body.Namespace)
		assert.Equal(t, tc.expectService, body.Service)
	}
}
