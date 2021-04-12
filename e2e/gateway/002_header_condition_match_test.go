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
	"context"
	"net/http"
	"testing"

	"github.com/projectcontour/contour/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestGatewayHeaderConditionMatch(t *testing.T) {
	// Not parallel because it defines a Gateway that lives in the projectcontour
	// namespace, which may conflict with other Gateway API tests.

	var (
		fx        = e2e.NewFramework(t)
		namespace = "gateway-002-header-condition-match"
	)

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo-header-exact")

	// Gateway
	gateway := &gatewayv1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "projectcontour", // TODO needs to be this to match default settings, but need to clean it up!
			Name:      "contour",
		},
		Spec: gatewayv1alpha1.GatewaySpec{
			GatewayClassName: "contour-class",
			Listeners: []gatewayv1alpha1.Listener{
				{
					Protocol: gatewayv1alpha1.HTTPProtocolType,
					Port:     gatewayv1alpha1.PortNumber(80),
					Routes: gatewayv1alpha1.RouteBindingSelector{
						Kind: "HTTPRoute",
						Namespaces: gatewayv1alpha1.RouteNamespaces{
							From: gatewayv1alpha1.RouteSelectAll,
						},
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "filter"},
						},
					},
				},
			},
		},
	}
	require.NoError(t, fx.Client.Create(context.TODO(), gateway))
	// TODO it'd be nice to have automatic object tracking
	defer fx.Client.Delete(context.TODO(), gateway)

	// HTTPRoute
	route := &gatewayv1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "http-filter-1",
			Labels:    map[string]string{"app": "filter"},
		},
		Spec: gatewayv1alpha1.HTTPRouteSpec{
			Hostnames: []gatewayv1alpha1.Hostname{"gatewayheaderconditions.projectcontour.io"},
			Rules: []gatewayv1alpha1.HTTPRouteRule{
				{
					Matches: []gatewayv1alpha1.HTTPRouteMatch{
						{
							Path: gatewayv1alpha1.HTTPPathMatch{
								Type:  gatewayv1alpha1.PathMatchPrefix,
								Value: "/",
							},
							Headers: &gatewayv1alpha1.HTTPHeaderMatch{
								Type: gatewayv1alpha1.HeaderMatchExact,
								Values: map[string]string{
									"My-Header": "Foo",
								},
							},
						},
					},
					ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
						{
							ServiceName: stringPtr("echo-header-exact"),
							Port:        portNumPtr(80),
						},
					},
				},
			},
		},
	}
	require.NoError(t, fx.Client.Create(context.TODO(), route))

	// TODO should wait until HTTPRoute has a status of valid

	type scenario struct {
		headers        map[string]string
		expectResponse int
		expectService  string
	}

	cases := []scenario{
		{
			headers:        map[string]string{"My-Header": "Foo"},
			expectResponse: 200,
			expectService:  "echo-header-exact",
		},
		{
			headers:        map[string]string{"My-Header": "NotFoo"},
			expectResponse: 404,
		},
		{
			headers:        map[string]string{"Other-Header": "Foo"},
			expectResponse: 404,
		},
	}

	for _, tc := range cases {
		setHeader := func(r *http.Request) {
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
		}

		res, ok := fx.HTTPRequestUntil(e2e.HasStatusCode(tc.expectResponse), "/", string(route.Spec.Hostnames[0]), setHeader)
		if !assert.Truef(t, ok, "did not get %d response", tc.expectResponse) {
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
