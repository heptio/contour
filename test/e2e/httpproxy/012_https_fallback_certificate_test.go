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

	. "github.com/onsi/ginkgo"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testHTTPSFallbackCertificate(namespace string) {
	Specify("requests with invalid SNI are accepted when a fallback cert is configured", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo-cert", "echo", "fallback-cert-echo.projectcontour.io")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "echo",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "fallback-cert-echo.projectcontour.io",
					TLS: &contourv1.TLS{
						SecretName:                "echo",
						EnableFallbackCertificate: true,
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
		f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		// Send a request that includes a valid SNI, confirm a 200 is
		// returned.
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected a 200 response code, got %d", res.StatusCode)

		assert.Equal(t, "echo", f.GetEchoResponseBody(res.Body).Service)

		// Send a request that does not include a valid SNI, confirm a
		// 200 is still returned since the fallback cert is used.
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host: p.Spec.VirtualHost.Fqdn,
			TLSConfigOpts: []func(*tls.Config){
				e2e.OptSetSNI("invalid-sni-should-use-fallback.projectcontour.io"),
			},
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected a 200 response code, got %d", res.StatusCode)
	})
}
