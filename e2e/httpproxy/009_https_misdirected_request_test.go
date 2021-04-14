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
	"crypto/tls"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testHTTPSMisdirectedRequest(t *testing.T, fx *e2e.Framework) {
	namespace := "009-https-misdirected-request"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.Echo.Create(namespace, "echo")
	fx.CreateSelfSignedCert(namespace, "echo-cert", "echo", "https-misdirected-request.projectcontour.io")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "echo",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "https-misdirected-request.projectcontour.io",
				TLS: &contourv1.TLS{
					SecretName: "echo",
				},
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
	fx.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

	res, ok := fx.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
		Host:      p.Spec.VirtualHost.Fqdn,
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "did not receive 200 response")

	assert.Equal(t, "echo", fx.GetEchoResponseBody(res.Body).Service)

	// Use a Host value that doesn't match the SNI value and verify
	// a 421 (Misdirected Request) is returned.
	res, ok = fx.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
		Host: "non-matching-host.projectcontour.io",
		TLSConfigOpts: []func(*tls.Config){
			e2e.OptSetSNI(p.Spec.VirtualHost.Fqdn),
		},
		Condition: e2e.HasStatusCode(421),
	})
	require.Truef(t, ok, "did not receive 421 (Misdirected Request) response")

	// The virtual host name is port-insensitive, so verify that we can
	// stuff any old port number in and still succeed.
	res, ok = fx.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
		Host: p.Spec.VirtualHost.Fqdn + ":9999",
		TLSConfigOpts: []func(*tls.Config){
			e2e.OptSetSNI(p.Spec.VirtualHost.Fqdn),
		},
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "did not receive 200 response")

	// Verify that the hostname match is case-insensitive.
	// The SNI server name match is still case sensitive,
	// see https://github.com/envoyproxy/envoy/issues/6199.
	res, ok = fx.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
		Host: "HTTPS-Misdirected-reQUest.projectcontour.io",
		TLSConfigOpts: []func(*tls.Config){
			e2e.OptSetSNI(p.Spec.VirtualHost.Fqdn),
		},
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "did not receive 200 response")
}
