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
	"crypto/tls"

	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func testTLSWildcardHost(namespace string) {
	Specify("wildcard hostname matching works with TLS", func() {
		t := f.T()
		hostSuffix := "wildcardhost.gateway.projectcontour.io"

		f.Fixtures.Echo.Deploy(namespace, "echo")

		route := &gatewayv1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "http-route-1",
				Labels:    map[string]string{"type": "secure"},
			},
			Spec: gatewayv1alpha1.HTTPRouteSpec{
				Hostnames: []gatewayv1alpha1.Hostname{"*.wildcardhost.gateway.projectcontour.io"},
				Gateways: &gatewayv1alpha1.RouteGateways{
					Allow: gatewayAllowTypePtr(gatewayv1alpha1.GatewayAllowAll),
				},
				Rules: []gatewayv1alpha1.HTTPRouteRule{
					{
						Matches: []gatewayv1alpha1.HTTPRouteMatch{{
							Path: &gatewayv1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayv1alpha1.PathMatchPrefix),
								Value: stringPtr("/"),
							},
						}},
						ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{{
							ServiceName: stringPtr("echo"),
							Port:        portNumPtr(80),
						}},
					},
				},
			},
		}
		f.CreateHTTPRouteAndWaitFor(route, httpRouteAdmitted)

		cases := []struct {
			hostname   string
			sni        string
			wantStatus int
		}{
			{
				hostname:   "random1." + hostSuffix,
				sni:        "random1." + hostSuffix,
				wantStatus: 200,
			},
			{
				hostname:   "random2." + hostSuffix,
				sni:        "random2." + hostSuffix,
				wantStatus: 200,
			},
			{
				hostname:   "a.random3." + hostSuffix,
				sni:        "a.random3." + hostSuffix,
				wantStatus: 404,
			},
			{
				hostname:   "random4." + hostSuffix,
				sni:        "other-random4." + hostSuffix,
				wantStatus: 421,
			},
			{
				hostname:   "random5." + hostSuffix,
				sni:        "a.random5." + hostSuffix,
				wantStatus: 421,
			},
			{
				hostname:   "random6." + hostSuffix + ":9999",
				sni:        "random6." + hostSuffix,
				wantStatus: 200,
			},
		}

		for _, tc := range cases {
			t.Logf("Making request with hostname=%s, sni=%s", tc.hostname, tc.sni)

			res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host: tc.hostname,
				TLSConfigOpts: []func(*tls.Config){
					e2e.OptSetSNI(tc.sni),
				},
				Condition: e2e.HasStatusCode(tc.wantStatus),
			})
			require.Truef(t, ok, "expected %d response code, got %d", tc.wantStatus, res.StatusCode)
		}
	})
}
