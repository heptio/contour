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

package httpproxy

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testHeaderConditionMatch(namespace string) {
	Specify("header match routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-header-present")
		f.Fixtures.Echo.Deploy(namespace, "echo-header-notpresent")
		f.Fixtures.Echo.Deploy(namespace, "echo-header-contains")
		f.Fixtures.Echo.Deploy(namespace, "echo-header-notcontains")
		f.Fixtures.Echo.Deploy(namespace, "echo-header-exact")
		f.Fixtures.Echo.Deploy(namespace, "echo-header-notexact")

		// This HTTPProxy tests everything except the "notpresent" match type,
		// which is tested separately below.
		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "header-conditions",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "headerconditions.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "echo-header-present",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Header: &contourv1.HeaderMatchCondition{
									Name:    "Target-Present",
									Present: true,
								},
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo-header-contains",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Header: &contourv1.HeaderMatchCondition{
									Name:     "Target-Contains",
									Contains: "ContainsValue",
								},
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo-header-notcontains",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Header: &contourv1.HeaderMatchCondition{
									Name:        "Target-NotContains",
									NotContains: "ContainsValue",
								},
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo-header-exact",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Header: &contourv1.HeaderMatchCondition{
									Name:  "Target-Exact",
									Exact: "ExactValue",
								},
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo-header-notexact",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Header: &contourv1.HeaderMatchCondition{
									Name:     "Target-NotExact",
									NotExact: "ExactValue",
								},
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		type scenario struct {
			headers        map[string]string
			expectResponse int
			expectService  string
		}

		cases := []scenario{
			{
				headers:        map[string]string{"Target-Present": "random"},
				expectResponse: 200,
				expectService:  "echo-header-present",
			},
			{
				headers:        map[string]string{"Target-Contains": "random"},
				expectResponse: 404,
			},
			{
				headers:        map[string]string{"Target-Contains": "ContainsValue"},
				expectResponse: 200,
				expectService:  "echo-header-contains",
			},
			{
				headers:        map[string]string{"Target-Contains": "xxx ContainsValue xxx"},
				expectResponse: 200,
				expectService:  "echo-header-contains",
			},
			{
				headers:        map[string]string{"Target-NotContains": "ContainsValue"},
				expectResponse: 404,
			},
			{
				headers:        map[string]string{"Target-NotContains": "xxx ContainsValue xxx"},
				expectResponse: 404,
			},
			{
				headers:        map[string]string{"Target-NotContains": "random"},
				expectResponse: 200,
				expectService:  "echo-header-notcontains",
			},
			{
				headers:        map[string]string{"Target-Exact": "random"},
				expectResponse: 404,
			},
			{
				headers:        map[string]string{"Target-Exact": "NotExactValue"},
				expectResponse: 404,
			},
			{
				headers:        map[string]string{"Target-Exact": "ExactValue"},
				expectResponse: 200,
				expectService:  "echo-header-exact",
			},
			{
				headers:        map[string]string{"Target-NotExact": "random"},
				expectResponse: 200,
				expectService:  "echo-header-notexact",
			},
			{
				headers:        map[string]string{"Target-NotExact": "NotExactValue"},
				expectResponse: 200,
				expectService:  "echo-header-notexact",
			},
			{
				headers:        map[string]string{"Target-NotExact": "ExactValue"},
				expectResponse: 404,
			},
		}

		for _, tc := range cases {
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host: p.Spec.VirtualHost.Fqdn,
				RequestOpts: []func(*http.Request){
					e2e.OptSetHeaders(tc.headers),
				},
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

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, tc.expectService, body.Service)
		}

		// Specifically test the "notpresent" match type in isolation.
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes = []contourv1.Route{
				{
					Services: []contourv1.Service{
						{
							Name: "echo-header-present",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Header: &contourv1.HeaderMatchCondition{
								Name:    "Target-Present",
								Present: true,
							},
						},
					},
				},
				{
					Services: []contourv1.Service{
						{
							Name: "echo-header-notpresent",
							Port: 80,
						},
					},
					Conditions: []contourv1.MatchCondition{
						{
							Header: &contourv1.HeaderMatchCondition{
								Name:       "Target-Present",
								NotPresent: true,
							},
						},
					},
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		cases = []scenario{
			{
				headers:        map[string]string{"Target-Present": "random"},
				expectResponse: 200,
				expectService:  "echo-header-present",
			},
			{
				headers:        nil,
				expectResponse: 200,
				expectService:  "echo-header-notpresent",
			},
		}
		for _, tc := range cases {
			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host: p.Spec.VirtualHost.Fqdn,
				RequestOpts: []func(*http.Request){
					e2e.OptSetHeaders(tc.headers),
				},
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

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, tc.expectService, body.Service)
		}
	})
}
