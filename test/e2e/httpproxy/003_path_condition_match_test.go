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
	. "github.com/onsi/ginkgo"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testPathConditionMatch(namespace string) {
	Specify("path match routing works", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo-slash-prefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-noprefix")
		f.Fixtures.Echo.Deploy(namespace, "echo-slash-default")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "path-conditions",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "pathconditions.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "echo-slash-prefix",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/path/prefix/",
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo-slash-noprefix",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/path/prefix",
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo-slash-default",
								Port: 80,
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		cases := map[string]string{
			"/":                "echo-slash-default",
			"/foo":             "echo-slash-default",
			"/path/prefix":     "echo-slash-noprefix",
			"/path/prefixfoo":  "echo-slash-noprefix",
			"/path/prefix/":    "echo-slash-prefix",
			"/path/prefix/foo": "echo-slash-prefix",
		}

		for path, expectedService := range cases {
			t.Logf("Querying %q, expecting service %q", path, expectedService)

			res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Path:      path,
				Condition: e2e.HasStatusCode(200),
			})
			if !assert.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode) {
				continue
			}

			body := f.GetEchoResponseBody(res.Body)
			assert.Equal(t, namespace, body.Namespace)
			assert.Equal(t, expectedService, body.Service)
		}
	})
}
