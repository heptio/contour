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
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testLocalRateLimitingVirtualHost(t *testing.T, fx *e2e.Framework) {
	namespace := "019-local-rate-limiting-vhost"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo")

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
	p, _ = fx.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

	// Wait until we get a 200 from the proxy confirming
	// the pods are up and serving traffic.
	_, ok := fx.HTTPRequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.True(t, ok, "did not get 200 response")

	// Add a local rate limit policy on the virtual host.
	p.Spec.VirtualHost.RateLimitPolicy = &contourv1.RateLimitPolicy{
		Local: &contourv1.LocalRateLimitPolicy{
			Requests: 1,
			Unit:     "hour",
		},
	}
	require.NoError(t, fx.Client.Update(context.TODO(), p))

	// Make a request against the proxy, confirm a 200 response
	// is returned since we're allowed one request per hour.
	//
	// TODO it'd be better to just make a single request.
	_, ok = fx.HTTPRequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.True(t, ok, "did not get 200 response")

	// Make another request against the proxy, confirm a 429 response
	// is now gotten since we've exceeded the rate limit.
	//
	// TODO it'd be better to just make a single request.
	_, ok = fx.HTTPRequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(429),
	})
	require.True(t, ok, "did not get 429 response")
}

func testLocalRateLimitingRoute(t *testing.T, fx *e2e.Framework) {
	namespace := "019-local-rate-limiting-route"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.CreateEchoWorkload(namespace, "echo")

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
	p, _ = fx.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

	// Wait until we get a 200 from the proxy confirming
	// the pods are up and serving traffic.
	_, ok := fx.HTTPRequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.True(t, ok, "did not get 200 response")

	// Add a local rate limit policy on the first route.
	p.Spec.Routes[0].RateLimitPolicy = &contourv1.RateLimitPolicy{
		Local: &contourv1.LocalRateLimitPolicy{
			Requests: 1,
			Unit:     "hour",
		},
	}
	require.NoError(t, fx.Client.Update(context.TODO(), p))

	// Make a request against the proxy, confirm a 200 response
	// is returned since we're allowed one request per hour.
	//
	// TODO it'd be better to just make a single request.
	_, ok = fx.HTTPRequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.True(t, ok, "did not get 200 response")

	// Make another request against the proxy, confirm a 429 response
	// is now gotten since we've exceeded the rate limit.
	//
	// TODO it'd be better to just make a single request.
	_, ok = fx.HTTPRequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(429),
	})
	require.True(t, ok, "did not get 429 response")

	// Make a request against the route that doesn't have rate limiting
	// to confirm we still get a 200 for that route.
	_, ok = fx.HTTPRequestUntil(&e2e.HTTPRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Path:      "/unlimited",
		Condition: e2e.HasStatusCode(200),
	})
	require.True(t, ok, "did not get 200 response for non-rate-limited route")
}
