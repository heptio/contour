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
	"testing"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var f = e2e.NewFramework(false)

func TestIngress(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ingress tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Deployment.EnsureResourcesForLocalContour())
})

var _ = AfterSuite(func() {
	f.DeleteNamespace(f.Deployment.Namespace.Name, true)
	gexec.CleanupBuildArtifacts()
})

var _ = Describe("Ingress", func() {
	var (
		contourCmd            *gexec.Session
		contourConfig         *config.Parameters
		contourConfigFile     string
		additionalContourArgs []string
	)

	BeforeEach(func() {
		// Contour config file contents, can be modified in nested
		// BeforeEach.
		contourConfig = &config.Parameters{}

		// Default contour serve command line arguments can be appended to in
		// nested BeforeEach.
		additionalContourArgs = []string{}
	})

	// JustBeforeEach is called after each of the nested BeforeEach are
	// called, so it is a final setup step before running a test.
	// A nested BeforeEach may have modified Contour config, so we wait
	// until here to start Contour.
	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Wait for Envoy to be healthy.
		require.NoError(f.T(), f.Deployment.WaitForEnvoyDaemonSetUpdated())
	})

	AfterEach(func() {
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	f.NamespacedTest("001-tls-wildcard-host", testTLSWildcardHost)

	f.NamespacedTest("002-ingress-ensure-v1beta1", testEnsureV1Beta1)

	f.NamespacedTest("backend-tls", func(namespace string) {
		Context("with backend tls", func() {
			BeforeEach(func() {
				// Top level issuer.
				selfSignedIssuer := &certmanagerv1.Issuer{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "selfsigned",
					},
					Spec: certmanagerv1.IssuerSpec{
						IssuerConfig: certmanagerv1.IssuerConfig{
							SelfSigned: &certmanagerv1.SelfSignedIssuer{},
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), selfSignedIssuer))

				// CA to sign backend certs with.
				caCertificate := &certmanagerv1.Certificate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "ca-cert",
					},
					Spec: certmanagerv1.CertificateSpec{
						IsCA: true,
						Usages: []certmanagerv1.KeyUsage{
							certmanagerv1.UsageSigning,
							certmanagerv1.UsageCertSign,
						},
						CommonName: "ca-cert",
						SecretName: "ca-cert",
						IssuerRef: certmanagermetav1.ObjectReference{
							Name: "selfsigned",
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), caCertificate))

				// Issuer based on CA to generate new certs with.
				basedOnCAIssuer := &certmanagerv1.Issuer{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "ca-issuer",
					},
					Spec: certmanagerv1.IssuerSpec{
						IssuerConfig: certmanagerv1.IssuerConfig{
							CA: &certmanagerv1.CAIssuer{
								SecretName: "ca-cert",
							},
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), basedOnCAIssuer))

				// Backend client cert, can use for upstream validation as well.
				backendClientCert := &certmanagerv1.Certificate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "backend-client-cert",
					},
					Spec: certmanagerv1.CertificateSpec{
						Usages: []certmanagerv1.KeyUsage{
							certmanagerv1.UsageClientAuth,
						},
						CommonName: "client",
						SecretName: "backend-client-cert",
						IssuerRef: certmanagermetav1.ObjectReference{
							Name: "ca-issuer",
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), backendClientCert))

				contourConfig.TLS = config.TLSParameters{
					ClientCertificate: config.NamespacedName{
						Namespace: namespace,
						Name:      "backend-client-cert",
					},
				}
			})

			testBackendTLS(namespace)
		})
	})
})
