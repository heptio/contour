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
	"crypto/tls"
	"crypto/x509"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testTCPRouteHTTPSTermination(t *testing.T, fx *e2e.Framework) {
	namespace := "008-tcp-route-https-termination"

	fx.CreateNamespace(namespace)
	defer fx.DeleteNamespace(namespace)

	fx.Fixtures.Echo.Create(namespace, "ingress-conformance-echo")
	fx.CreateSelfSignedCert(namespace, "echo-cert", "echo-cert", "tcp-route-https-termination.projectcontour.io")

	p := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "echo-tcpproxy",
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "tcp-route-https-termination.projectcontour.io",
				TLS: &contourv1.TLS{
					SecretName: "echo-cert",
				},
			},
			TCPProxy: &contourv1.TCPProxy{
				Services: []contourv1.Service{
					{
						Name: "ingress-conformance-echo",
						Port: 80,
					},
				},
			},
		},
	}
	fx.CreateHTTPProxyAndWaitFor(p, httpProxyValid)

	certSecret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: namespace, Name: "echo-cert"}
	require.NoError(t, fx.Client.Get(context.TODO(), key, certSecret))

	_, ok := fx.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
		Host: p.Spec.VirtualHost.Fqdn,
		TLSConfigOpts: []func(*tls.Config){
			func(c *tls.Config) {
				certPool := x509.NewCertPool()
				certPool.AppendCertsFromPEM(certSecret.Data["ca.crt"])

				c.RootCAs = certPool
				c.InsecureSkipVerify = false
			},
		},
		Condition: e2e.HasStatusCode(200),
	})
	require.Truef(t, ok, "did not receive 200 response")
}