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
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testDynamicHeaders(namespace string) {
	Specify("dynamic request and response headers can be set for route services", func() {
		t := f.T()

		f.Fixtures.Echo.Deploy(namespace, "ingress-conformance-echo")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "dynamic-headers",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "dynamicheaders.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name:                  "ingress-conformance-echo",
								Port:                  80,
								RequestHeadersPolicy:  &contourv1.HeadersPolicy{},
								ResponseHeadersPolicy: &contourv1.HeadersPolicy{},
							},
						},
					},
				},
			},
		}

		requestHeaders := map[string]string{
			"Request-Header":                  "foo",
			"X-App-Weight":                    "100%",
			"X-Envoy-Hostname":                "%HOSTNAME%",
			"X-Envoy-Unknown":                 "%UNKNOWN%",
			"X-Envoy-Upstream-Remote-Address": "%UPSTREAM_REMOTE_ADDRESS%",
			"X-Request-Host":                  "%REQ(Host)%",
			"X-Request-Missing-Header":        "%REQ(Missing-Header)%ook",
			"X-Host-Protocol":                 "%REQ(Host)% - %PROTOCOL%",
			"X-Dynamic-Header-1":              "%DOWNSTREAM_REMOTE_ADDRESS%",
			"X-Dynamic-Header-2":              "%DOWNSTREAM_REMOTE_ADDRESS_WITHOUT_PORT%",
			"X-Dynamic-Header-3":              "%DOWNSTREAM_LOCAL_ADDRESS%",
			"X-Dynamic-Header-4":              "%DOWNSTREAM_LOCAL_ADDRESS_WITHOUT_PORT%",
			"X-Dynamic-Header-5":              "%DOWNSTREAM_LOCAL_PORT%",
			"X-Dynamic-Header-6":              "%DOWNSTREAM_LOCAL_URI_SAN%",
			"X-Dynamic-Header-7":              "%DOWNSTREAM_PEER_URI_SAN%",
			"X-Dynamic-Header-8":              "%DOWNSTREAM_LOCAL_SUBJECT%",
			"X-Dynamic-Header-9":              "%DOWNSTREAM_PEER_SUBJECT%",
			"X-Dynamic-Header-10":             "%DOWNSTREAM_PEER_ISSUER%",
			"X-Dynamic-Header-11":             "%DOWNSTREAM_TLS_SESSION_ID%",
			"X-Dynamic-Header-12":             "%DOWNSTREAM_TLS_CIPHER%",
			"X-Dynamic-Header-13":             "%DOWNSTREAM_TLS_VERSION%",
			"X-Dynamic-Header-14":             "%DOWNSTREAM_PEER_FINGERPRINT_256%",
			"X-Dynamic-Header-15":             "%DOWNSTREAM_PEER_FINGERPRINT_1%",
			"X-Dynamic-Header-16":             "%DOWNSTREAM_PEER_SERIAL%",
			"X-Dynamic-Header-17":             "%DOWNSTREAM_PEER_CERT%",
			"X-Dynamic-Header-18":             "%DOWNSTREAM_PEER_CERT_V_START%",
			"X-Dynamic-Header-19":             "%DOWNSTREAM_PEER_CERT_V_END%",
			"X-Dynamic-Header-20":             "%HOSTNAME%",
			"X-Dynamic-Header-21":             "%PROTOCOL%",
			"X-Dynamic-Header-22":             "%UPSTREAM_REMOTE_ADDRESS%",
			"X-Dynamic-Header-23":             "%RESPONSE_FLAGS%",
			"X-Dynamic-Header-24":             "%RESPONSE_CODE_DETAILS%",
			"X-Contour-Namespace":             "%CONTOUR_NAMESPACE%",
			"X-Contour-Service":               "%CONTOUR_SERVICE_NAME%:%CONTOUR_SERVICE_PORT%",
		}
		for k, v := range requestHeaders {
			hv := contourv1.HeaderValue{
				Name:  k,
				Value: v,
			}
			p.Spec.Routes[0].Services[0].RequestHeadersPolicy.Set = append(p.Spec.Routes[0].Services[0].RequestHeadersPolicy.Set, hv)
		}

		responseHeaders := map[string]string{
			"Response-Header":                 "bar",
			"X-App-Weight":                    "100%",
			"X-Envoy-Hostname":                "%HOSTNAME%",
			"X-Envoy-Unknown":                 "%UNKNOWN%",
			"X-Envoy-Upstream-Remote-Address": "%UPSTREAM_REMOTE_ADDRESS%",
			"X-Request-Host":                  "%REQ(Host)%",
			"X-Request-Missing-Header":        "%REQ(Missing-Header)%ook",
			"X-Host-Protocol":                 "%REQ(Host)% - %PROTOCOL%",
			"X-Dynamic-Header-1":              "%DOWNSTREAM_REMOTE_ADDRESS%",
			"X-Dynamic-Header-2":              "%DOWNSTREAM_REMOTE_ADDRESS_WITHOUT_PORT%",
			"X-Dynamic-Header-3":              "%DOWNSTREAM_LOCAL_ADDRESS%",
			"X-Dynamic-Header-4":              "%DOWNSTREAM_LOCAL_ADDRESS_WITHOUT_PORT%",
			"X-Dynamic-Header-5":              "%DOWNSTREAM_LOCAL_PORT%",
			"X-Dynamic-Header-6":              "%DOWNSTREAM_LOCAL_URI_SAN%",
			"X-Dynamic-Header-7":              "%DOWNSTREAM_PEER_URI_SAN%",
			"X-Dynamic-Header-8":              "%DOWNSTREAM_LOCAL_SUBJECT%",
			"X-Dynamic-Header-9":              "%DOWNSTREAM_PEER_SUBJECT%",
			"X-Dynamic-Header-10":             "%DOWNSTREAM_PEER_ISSUER%",
			"X-Dynamic-Header-11":             "%DOWNSTREAM_TLS_SESSION_ID%",
			"X-Dynamic-Header-12":             "%DOWNSTREAM_TLS_CIPHER%",
			"X-Dynamic-Header-13":             "%DOWNSTREAM_TLS_VERSION%",
			"X-Dynamic-Header-14":             "%DOWNSTREAM_PEER_FINGERPRINT_256%",
			"X-Dynamic-Header-15":             "%DOWNSTREAM_PEER_FINGERPRINT_1%",
			"X-Dynamic-Header-16":             "%DOWNSTREAM_PEER_SERIAL%",
			"X-Dynamic-Header-17":             "%DOWNSTREAM_PEER_CERT%",
			"X-Dynamic-Header-18":             "%DOWNSTREAM_PEER_CERT_V_START%",
			"X-Dynamic-Header-19":             "%DOWNSTREAM_PEER_CERT_V_END%",
			"X-Dynamic-Header-20":             "%HOSTNAME%",
			"X-Dynamic-Header-21":             "%PROTOCOL%",
			"X-Dynamic-Header-22":             "%UPSTREAM_REMOTE_ADDRESS%",
			"X-Dynamic-Header-23":             "%RESPONSE_FLAGS%",
			"X-Dynamic-Header-24":             "%RESPONSE_CODE_DETAILS%",
		}
		for k, v := range responseHeaders {
			hv := contourv1.HeaderValue{
				Name:  k,
				Value: v,
			}
			p.Spec.Routes[0].Services[0].ResponseHeadersPolicy.Set = append(p.Spec.Routes[0].Services[0].ResponseHeadersPolicy.Set, hv)
		}

		f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)

		body := f.GetEchoResponseBody(res.Body)

		// Both request and response headers should match
		// all of these assertions.
		headerSets := map[string]http.Header{
			"request":  body.RequestHeaders,
			"response": res.Headers,
		}
		for name, headers := range headerSets {
			t.Logf("Checking %s headers", name)

			// Check simple percentage escape
			assert.Equal(t, "100%", headers.Get("X-App-Weight"))

			// Check known good Envoy dynamic header value
			assert.True(t, strings.HasPrefix(headers.Get("X-Envoy-Hostname"), "envoy-"), "X-Envoy-Hostname does not start with 'envoy-'")

			// Check unknown Envoy dynamic header value
			assert.Equal(t, "%UNKNOWN%", headers.Get("X-Envoy-Unknown"))

			// Check valid Envoy REQ value for header that exists
			assert.Equal(t, body.Host, headers.Get("X-Request-Host"))

			// Check invalid Envoy REQ value for header that does not exist
			assert.Equal(t, "ook", headers.Get("X-Request-Missing-Header"))

			// Check header value with dynamic and non-dynamic content and multiple dynamic fields
			assert.Equal(t, body.Host+" - HTTP/1.1", headers.Get("X-Host-Protocol"))
		}

		// Check dynamic service headers are populated as expected (only on request headers)
		assert.Equal(t, "ingress-conformance-echo:80", body.RequestHeaders.Get("X-Contour-Service"))
	})
}
