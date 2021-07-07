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

	. "github.com/onsi/ginkgo"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testLocalRateLimitingVirtualHost(namespace string) {
	Specify("local rate limiting can be specified at the virtualhost", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "vhostlocalratelimit",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "vhostlocalratelimit.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
				},
			},
		}
		p, _ = f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		// Wait until we get a 200 from the proxy confirming
		// the pods are up and serving traffic.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Add a local rate limit policy on the virtual host.
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.VirtualHost.RateLimitPolicy = &contourv1.RateLimitPolicy{
				Local: &contourv1.LocalRateLimitPolicy{
					Requests: 1,
					Unit:     "hour",
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Make a request against the proxy, confirm a 200 response
		// is returned since we're allowed one request per hour.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Make another request against the proxy, confirm a 429 response
		// is now gotten since we've exceeded the rate limit.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
		})
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)
	})
}

func testLocalRateLimitingRoute(namespace string) {
	Specify("local rate limits can be specified on a route", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "routelocalratelimit",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "routelocalratelimit.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
					},
					{
						Services: []contourv1.Service{
							{
								Name: "echo",
								Port: 80,
							},
						},
						Conditions: []contourv1.MatchCondition{
							{
								Prefix: "/unlimited",
							},
						},
					},
				},
			},
		}
		p, _ = f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		// Wait until we get a 200 from the proxy confirming
		// the pods are up and serving traffic.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Add a local rate limit policy on the first route.
		require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(p), p); err != nil {
				return err
			}

			p.Spec.Routes[0].RateLimitPolicy = &contourv1.RateLimitPolicy{
				Local: &contourv1.LocalRateLimitPolicy{
					Requests: 1,
					Unit:     "hour",
				},
			}

			return f.Client.Update(context.TODO(), p)
		}))

		// Make a request against the proxy, confirm a 200 response
		// is returned since we're allowed one request per hour.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		// Make another request against the proxy, confirm a 429 response
		// is now gotten since we've exceeded the rate limit.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(429),
		})
		require.Truef(t, ok, "expected 429 response code, got %d", res.StatusCode)

		// Make a request against the route that doesn't have rate limiting
		// to confirm we still get a 200 for that route.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Path:      "/unlimited",
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code for non-rate-limited route, got %d", res.StatusCode)
	})
}
