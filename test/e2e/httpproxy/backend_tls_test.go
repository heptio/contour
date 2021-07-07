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
	"encoding/json"
	"time"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testBackendTLS(namespace string) {
	Specify("mTLS to backends can be configured", func() {
		// Backend server cert signed by CA.
		backendServerCert := &certmanagerv1.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "backend-server-cert",
			},
			Spec: certmanagerv1.CertificateSpec{
				Usages: []certmanagerv1.KeyUsage{
					certmanagerv1.UsageServerAuth,
				},
				CommonName: "echo-secure",
				DNSNames:   []string{"echo-secure"},
				SecretName: "backend-server-cert",
				IssuerRef: certmanagermetav1.ObjectReference{
					Name: "ca-issuer",
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), backendServerCert))
		f.Fixtures.EchoSecure.Deploy(namespace, "echo-secure")

		p := &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "backend-tls",
			},
			Spec: contourv1.HTTPProxySpec{
				VirtualHost: &contourv1.VirtualHost{
					Fqdn: "backend-tls.projectcontour.io",
				},
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: "echo-secure",
								Port: 443,
								UpstreamValidation: &contourv1.UpstreamValidation{
									CACertificate: "backend-client-cert",
									SubjectName:   "echo-secure",
								},
							},
						},
					},
				},
			},
		}
		f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid)

		type responseTLSDetails struct {
			TLS struct {
				PeerCertificates []string
			}
		}

		// Send HTTP request, we will check backend connection was over HTTPS.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(f.T(), res, "request never succeeded")
		require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)

		// Get cert presented to backend app.
		tlsInfo := new(responseTLSDetails)
		require.NoError(f.T(), json.Unmarshal(res.Body, tlsInfo))
		require.Len(f.T(), tlsInfo.TLS.PeerCertificates, 1)

		// Get value of client cert Envoy should have presented.
		clientSecretKey := client.ObjectKey{Namespace: namespace, Name: "backend-client-cert"}
		clientSecret := &corev1.Secret{}
		require.NoError(f.T(), f.Client.Get(context.TODO(), clientSecretKey, clientSecret))

		assert.Equal(f.T(), tlsInfo.TLS.PeerCertificates[0], string(clientSecret.Data["tls.crt"]))

		// Delete client cert so it is rotated.
		oldUID := clientSecret.UID
		require.NoError(f.T(), f.Client.Delete(context.TODO(), clientSecret))
		// Make sure the cert is rotated.
		require.Eventually(f.T(), func() bool {
			if err := f.Client.Get(context.TODO(), clientSecretKey, clientSecret); err != nil {
				return false
			}
			return clientSecret.UID != oldUID
		}, time.Second*10, time.Millisecond*50)

		// Send HTTP request again.
		res, ok = f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      p.Spec.VirtualHost.Fqdn,
			Condition: e2e.HasStatusCode(200),
		})
		require.NotNil(f.T(), res, "request never succeeded")
		require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)

		// Get cert presented to backend app.
		tlsInfo = new(responseTLSDetails)
		require.NoError(f.T(), json.Unmarshal(res.Body, tlsInfo))
		require.Len(f.T(), tlsInfo.TLS.PeerCertificates, 1)

		// Get value of client cert Envoy should have presented.
		require.NoError(f.T(), f.Client.Get(context.TODO(), clientSecretKey, clientSecret))

		assert.Equal(f.T(), tlsInfo.TLS.PeerCertificates[0], string(clientSecret.Data["tls.crt"]))
	})
}
