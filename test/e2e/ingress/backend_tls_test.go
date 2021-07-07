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

package ingress

import (
	"context"
	"encoding/json"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testBackendTLS(namespace string) {
	Specify("simple TLS to backends can be configured", func() {
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

		i := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "backend-tls",
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{
						Host: "backend-tls.ingress.projectcontour.io",
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										PathType: e2e.IngressPathTypePtr(networkingv1.PathTypePrefix),
										Path:     "/",
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: "echo-secure",
												Port: networkingv1.ServiceBackendPort{
													Number: 443,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		require.NoError(f.T(), f.Client.Create(context.TODO(), i))

		type responseTLSDetails struct {
			TLS struct {
				PeerCertificates []string
			}
		}

		// Send HTTP request, we will check backend connection was over HTTPS.
		res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
			Host:      "backend-tls.ingress.projectcontour.io",
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
	})
}
