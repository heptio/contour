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

package dag

import (
	"errors"
	"testing"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestKubernetesCacheInsert(t *testing.T) {
	tests := map[string]struct {
		pre  []interface{}
		obj  interface{}
		want bool
	}{
		"insert secret": {
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			want: false,
		},
		"insert secret w/ blank ca.crt": {
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					CACertificateKey:    []byte(""),
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				},
			},
			want: true,
		},
		"insert CA secret w/ explanatory text": {
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(fixture.CERTIFICATE_WITH_TEXT),
				},
			},
			want: true,
		},
		"insert CA bundle secret w/ non-PEM data": {
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeOpaque,
				Data: caBundleData(fixture.CERTIFICATE, fixture.CERTIFICATE, fixture.CERTIFICATE, fixture.CERTIFICATE),
			},
			want: true,
		},
		"insert CA bundle secret w/ non-PEM data and no certificates": {
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeOpaque,
				Data: caBundleData(),
			},
			want: false,
		},

		"insert secret referenced by ingress": {
			pre: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "www",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							SecretName: "secret",
						}},
					},
				},
			},
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			want: true,
		},
		"insert secret referenced by ingress with multiple pem blocks": {
			pre: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "www",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							SecretName: "secret",
						}},
					},
				},
			},
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.EC_CERTIFICATE, fixture.EC_PRIVATE_KEY),
			},
			want: true,
		},
		"insert secret w/ wrong type referenced by ingress": {
			pre: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "www",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							SecretName: "secret",
						}},
					},
				},
			},
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: "banana",
			},
			want: false,
		},
		"insert secret referenced by ingress via tls delegation": {
			pre: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "www",
						Namespace: "extra",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							SecretName: "default/secret",
						}},
					},
				},
				&contour_api_v1.TLSCertificateDelegation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "delegation",
						Namespace: "default",
					},
					Spec: contour_api_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_api_v1.CertificateDelegation{{
							SecretName: "secret",
							TargetNamespaces: []string{
								"extra",
							},
						}},
					},
				},
			},
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			want: true,
		},
		"insert secret referenced by ingress via wildcard tls delegation": {
			pre: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "www",
						Namespace: "extra",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							SecretName: "default/secret",
						}},
					},
				},

				&contour_api_v1.TLSCertificateDelegation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "delegation",
						Namespace: "default",
					},
					Spec: contour_api_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_api_v1.CertificateDelegation{{
							SecretName: "secret",
							TargetNamespaces: []string{
								"*",
							},
						}},
					},
				},
			},
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			want: true,
		},
		"insert secret referenced by httpproxy": {
			pre: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
					},
				},
			},
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			want: true,
		},
		"insert secret referenced by httpproxy via tls delegation": {
			pre: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "extra",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							TLS: &contour_api_v1.TLS{
								SecretName: "default/secret",
							},
						},
					},
				},
				&contour_api_v1.TLSCertificateDelegation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "delegation",
						Namespace: "default",
					},
					Spec: contour_api_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_api_v1.CertificateDelegation{{
							SecretName: "secret",
							TargetNamespaces: []string{
								"extra",
							},
						}},
					},
				},
			},
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			want: true,
		},
		"insert secret referenced by httpproxy via wildcard tls delegation": {
			pre: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "extra",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							TLS: &contour_api_v1.TLS{
								SecretName: "default/secret",
							},
						},
					},
				},
				&contour_api_v1.TLSCertificateDelegation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "delegation",
						Namespace: "default",
					},
					Spec: contour_api_v1.TLSCertificateDelegationSpec{
						Delegations: []contour_api_v1.CertificateDelegation{{
							SecretName: "secret",
							TargetNamespaces: []string{
								"*",
							},
						}},
					},
				},
			},
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			want: true,
		},
		"insert certificate secret": {
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ca",
					Namespace: "default",
				},
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(fixture.CERTIFICATE),
				},
			},
			// TODO(dfc) this should be false because the CA secret is
			// not referenced, but computing its reference duplicates the
			// work done rebuilding the dag so for the moment assume that
			// any CA secret causes a rebuild.
			want: true,
		},
		"insert certificate secret referenced by httpproxy": {
			pre: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-com",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "kuard",
								Port: 8080,
								UpstreamValidation: &contour_api_v1.UpstreamValidation{
									CACertificate: "ca",
									SubjectName:   "example.com",
								},
							}},
						}},
					},
				},
			},
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ca",
					Namespace: "default",
				},
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					CACertificateKey: []byte(fixture.CERTIFICATE),
				},
			},
			want: true,
		},
		"insert ingress class correct name": {
			obj: &networking_v1.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "contour",
				},
			},
			want: true,
		},
		"insert ingress class incorrect name": {
			obj: &networking_v1.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "notagoodclass",
				},
			},
			want: false,
		},
		"insert ingressv1 empty ingress class": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "correct",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert ingressv1 incorrect ingress class name": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
				},
				Spec: networking_v1.IngressSpec{
					IngressClassName: pointer.StringPtr("nginx"),
				},
			},
			want: false,
		},
		"insert ingressv1 explicit ingress class name": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "explicit",
					Namespace: "default",
				},
				Spec: networking_v1.IngressSpec{
					IngressClassName: pointer.StringPtr("contour"),
				},
			},
			want: true,
		},
		"insert ingressv1 incorrect kubernetes.io/ingress.class": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			want: false,
		},
		"insert ingressv1 incorrect projectcontour.io/ingress.class": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": "nginx",
					},
				},
			},
			want: false,
		},
		"insert ingressv1 explicit kubernetes.io/ingress.class": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "explicit",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": annotation.DEFAULT_INGRESS_CLASS_NAME,
					},
				},
			},
			want: true,
		},
		"insert ingressv1 explicit projectcontour.io/ingress.class": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "explicit",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": annotation.DEFAULT_INGRESS_CLASS_NAME,
					},
				},
			},
			want: true,
		},
		"insert ingressv1 projectcontour.io ingress class annotation overrides kubernetes.io incorrect": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "override",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": "nginx",
						"kubernetes.io/ingress.class":     annotation.DEFAULT_INGRESS_CLASS_NAME,
					},
				},
			},
			want: false,
		},
		"insert ingressv1 projectcontour.io ingress class annotation overrides kubernetes.io correct": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "override",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": annotation.DEFAULT_INGRESS_CLASS_NAME,
						"kubernetes.io/ingress.class":     "nginx",
					},
				},
			},
			want: true,
		},
		"insert ingressv1 ingress class annotation overrides spec incorrect": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "override",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": "nginx",
					},
				},
				Spec: networking_v1.IngressSpec{
					IngressClassName: pointer.StringPtr("contour"),
				},
			},
			want: false,
		},
		"insert ingressv1 ingress class annotation overrides spec correct": {
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "override",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": annotation.DEFAULT_INGRESS_CLASS_NAME,
					},
				},
				Spec: networking_v1.IngressSpec{
					IngressClassName: pointer.StringPtr("nginx"),
				},
			},
			want: true,
		},
		"insert httpproxy empty ingress annotation": {
			obj: &contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kuard",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert httpproxy incorrect kubernetes.io/ingress.class": {
			obj: &contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			want: false,
		},
		"insert httpproxy incorrect projectcontour.io/ingress.class": {
			obj: &contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": "nginx",
					},
				},
			},
			want: false,
		},
		"insert httpproxy explicit kubernetes.io/ingress.class": {
			obj: &contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kuard",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": annotation.DEFAULT_INGRESS_CLASS_NAME,
					},
				},
			},
			want: true,
		},
		"insert httpproxy explicit projectcontour.io/ingress.class": {
			obj: &contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kuard",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontours.io/ingress.class": annotation.DEFAULT_INGRESS_CLASS_NAME,
					},
				},
			},
			want: true,
		},
		"insert tls contour_api_v1/v1.certificatedelegation": {
			obj: &contour_api_v1.TLSCertificateDelegation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delegate",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert httpproxy": {
			obj: &contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpproxy",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert unknown": {
			obj:  "not an object",
			want: false,
		},
		"insert service": {
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
			},
			want: false,
		},
		"insert service referenced by ingress backend": {
			pre: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "www",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: &networking_v1.IngressBackend{
							Service: &networking_v1.IngressServiceBackend{
								Name: "service",
							},
						},
					},
				},
			},
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert service in different namespace": {
			pre: []interface{}{
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "www",
						Namespace: "kube-system",
					},
					Spec: networking_v1.IngressSpec{
						DefaultBackend: &networking_v1.IngressBackend{
							Service: &networking_v1.IngressServiceBackend{
								Name: "service",
							},
						},
					},
				},
			},
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
			},
			want: false,
		},
		"insert service referenced by httpproxy": {
			pre: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "service",
							}},
						}},
					},
				},
			},
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert service referenced by httpproxy tcpproxy": {
			pre: []interface{}{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						TCPProxy: &contour_api_v1.TCPProxy{
							Services: []contour_api_v1.Service{{
								Name: "service",
							}},
						},
					},
				},
			},
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert namespace": {
			obj: &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "namespace",
					Namespace: "default",
				},
			},
			want: true,
		},
		// invalid gatewayclass test case is unneeded since the controller
		// uses a predicate to filter events before they're given to the EventHandler.
		"insert valid gatewayclass": {
			obj: &gatewayapi_v1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "contour",
				},
			},
			want: true,
		},
		"insert gateway-api Gateway": {
			obj: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
			},
			want: true,
		},
		"insert gateway-api HTTPRoute": {
			obj: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert gateway-api TCPRoute": {
			obj: &gatewayapi_v1alpha1.TCPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcproute",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert gateway-api UDPRoute": {
			obj: &gatewayapi_v1alpha1.UDPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "udproute",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert gateway-api TLSRoute": {
			obj: &gatewayapi_v1alpha1.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tlsroute",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert gateway-api BackendPolicy": {
			obj: &gatewayapi_v1alpha1.BackendPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backendpolicy",
					Namespace: "default",
				},
			},
			want: true,
		},
		"insert extension service": {
			obj: &contour_api_v1alpha1.ExtensionService{
				ObjectMeta: fixture.ObjectMeta("default/extension"),
			},
			want: true,
		},
		"insert secret that is referred by configuration file": {
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secretReferredByConfigFile",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
			},
			want: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cache := KubernetesCache{
				ConfiguredSecretRefs: []*types.NamespacedName{
					{Name: "secretReferredByConfigFile", Namespace: "default"}},
				FieldLogger: fixture.NewTestLogger(t),
			}
			for _, p := range tc.pre {
				cache.Insert(p)
			}
			got := cache.Insert(tc.obj)
			assert.Equalf(t, tc.want, got, "Insert failed for object %v ", tc.obj)
		})
	}
}

func TestKubernetesCacheRemove(t *testing.T) {
	cache := func(objs ...interface{}) *KubernetesCache {
		cache := KubernetesCache{
			FieldLogger: fixture.NewTestLogger(t),
		}
		for _, o := range objs {
			cache.Insert(o)
		}
		return &cache
	}

	tests := map[string]struct {
		cache *KubernetesCache
		obj   interface{}
		want  bool
	}{
		"remove secret": {
			cache: cache(&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					v1.TLSCertKey:       []byte(fixture.CERTIFICATE),
					v1.TLSPrivateKeyKey: []byte(fixture.RSA_PRIVATE_KEY),
				},
			}),
			obj: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Type: v1.SecretTypeTLS,
			},
			want: true,
		},
		"remove service": {
			cache: cache(&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
			}),
			obj: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove namespace": {
			cache: cache(&v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "namespace",
					Namespace: "default",
				},
			}),
			obj: &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "namespace",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove ingress class correct name": {
			cache: cache(&networking_v1.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "contour",
				},
			}),
			obj: &networking_v1.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "contour",
				},
			},
			want: true,
		},
		"remove ingress class wrong name": {
			cache: cache(&networking_v1.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "contour",
				},
			}),
			obj: &networking_v1.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "somethingelse",
				},
			},
			want: false,
		},
		"remove ingress": {
			cache: cache(&networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "default",
				},
			}),
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove ingressv1": {
			cache: cache(&networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "default",
				},
			}),
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove ingress incorrect ingressclass": {
			cache: cache(&networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			}),
			obj: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			want: false,
		},
		"remove httpproxy": {
			cache: cache(&contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpproxy",
					Namespace: "default",
				},
			}),
			obj: &contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpproxy",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove httpproxy incorrect ingressclass": {
			cache: cache(&contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpproxy",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			}),
			obj: &contour_api_v1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpproxy",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			want: false,
		},
		"remove gatewayclass": {
			cache: cache(&gatewayapi_v1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "contour",
				},
			}),
			obj: &gatewayapi_v1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "contour",
				},
			},
			want: true,
		},
		"remove gateway-api Gateway": {
			cache: cache(&gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
			}),
			obj: &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "contour",
					Namespace: "projectcontour",
				},
			},
			want: true,
		},
		"remove gateway-api HTTPRoute": {
			cache: cache(&gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute",
					Namespace: "default",
				},
			}),
			obj: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove gateway-api TCPRoute": {
			cache: cache(&gatewayapi_v1alpha1.TCPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcproute",
					Namespace: "default",
				},
			}),
			obj: &gatewayapi_v1alpha1.TCPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcproute",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove gateway-api UDPRoute": {
			cache: cache(&gatewayapi_v1alpha1.UDPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "udproute",
					Namespace: "default",
				},
			}),
			obj: &gatewayapi_v1alpha1.UDPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "udproute",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove gateway-api TLSRoute": {
			cache: cache(&gatewayapi_v1alpha1.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tlsroute",
					Namespace: "default",
				},
			}),
			obj: &gatewayapi_v1alpha1.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tlsroute",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove gateway-api BackendPolicy": {
			cache: cache(&gatewayapi_v1alpha1.BackendPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backendpolicy",
					Namespace: "default",
				},
			}),
			obj: &gatewayapi_v1alpha1.BackendPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backendpolicy",
					Namespace: "default",
				},
			},
			want: true,
		},
		"remove extension service": {
			cache: cache(&contour_api_v1alpha1.ExtensionService{
				ObjectMeta: fixture.ObjectMeta("default/extension"),
			}),
			obj: &contour_api_v1alpha1.ExtensionService{
				ObjectMeta: fixture.ObjectMeta("default/extension"),
			},
			want: true,
		},
		"remove unknown": {
			cache: cache("not an object"),
			obj:   "not an object",
			want:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.cache.Remove(tc.obj)
			assert.Equalf(t, tc.want, got, "Remove failed for object %v ", tc.obj)
		})
	}
}

func TestLookupService(t *testing.T) {
	cache := func(objs ...interface{}) *KubernetesCache {
		cache := KubernetesCache{
			FieldLogger: fixture.NewTestLogger(t),
		}
		for _, o := range objs {
			cache.Insert(o)
		}
		return &cache
	}

	service := func(ns, name string, ports ...v1.ServicePort) *v1.Service {
		return &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
			Spec: v1.ServiceSpec{
				Ports: ports,
			},
		}
	}

	port := func(name string, port int32, protocol v1.Protocol) v1.ServicePort {
		return v1.ServicePort{
			Name:     name,
			Port:     port,
			Protocol: protocol,
		}
	}

	tests := map[string]struct {
		cache    *KubernetesCache
		meta     types.NamespacedName
		port     intstr.IntOrString
		wantSvc  *v1.Service
		wantPort v1.ServicePort
		wantErr  error
	}{
		"service and port exist with valid service protocol, lookup by port num": {
			cache:    cache(service("default", "service-1", port("http", 80, v1.ProtocolTCP))),
			meta:     types.NamespacedName{Namespace: "default", Name: "service-1"},
			port:     intstr.FromInt(80),
			wantSvc:  service("default", "service-1", port("http", 80, v1.ProtocolTCP)),
			wantPort: port("http", 80, v1.ProtocolTCP),
		},
		"service and port exist with valid service protocol, lookup by port name": {
			cache:    cache(service("default", "service-1", port("http", 80, v1.ProtocolTCP))),
			meta:     types.NamespacedName{Namespace: "default", Name: "service-1"},
			port:     intstr.FromString("http"),
			wantSvc:  service("default", "service-1", port("http", 80, v1.ProtocolTCP)),
			wantPort: port("http", 80, v1.ProtocolTCP),
		},
		"service and port exist with valid service protocol, lookup by wrong port num": {
			cache:   cache(service("default", "service-1", port("http", 80, v1.ProtocolTCP))),
			meta:    types.NamespacedName{Namespace: "default", Name: "service-1"},
			port:    intstr.FromInt(9999),
			wantErr: errors.New(`port "9999" on service "default/service-1" not matched`),
		},
		"service and port exist with valid service protocol, lookup by wrong port name": {
			cache:   cache(service("default", "service-1", port("http", 80, v1.ProtocolTCP))),
			meta:    types.NamespacedName{Namespace: "default", Name: "service-1"},
			port:    intstr.FromString("wrong-port-name"),
			wantErr: errors.New(`port "wrong-port-name" on service "default/service-1" not matched`),
		},
		"service and port exist, invalid service protocol": {
			cache:   cache(service("default", "service-1", port("http", 80, v1.ProtocolUDP))),
			meta:    types.NamespacedName{Namespace: "default", Name: "service-1"},
			port:    intstr.FromString("http"),
			wantSvc: service("default", "service-1", port("http", 80, v1.ProtocolTCP)),
			wantErr: errors.New(`unsupported service protocol "UDP"`),
		},
		"service does not exist": {
			cache:   cache(service("default", "service-1", port("http", 80, v1.ProtocolTCP))),
			meta:    types.NamespacedName{Namespace: "default", Name: "nonexistent-service"},
			port:    intstr.FromInt(80),
			wantErr: errors.New(`service "default/nonexistent-service" not found`),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotSvc, gotPort, gotErr := tc.cache.LookupService(tc.meta, tc.port)

			switch {
			case tc.wantErr != nil:
				require.Error(t, gotErr)
				assert.EqualError(t, tc.wantErr, gotErr.Error())
			default:
				assert.Nil(t, gotErr)
				assert.Equal(t, tc.wantSvc, gotSvc)
				assert.Equal(t, tc.wantPort, gotPort)
			}
		})
	}
}

func TestServiceTriggersRebuild(t *testing.T) {

	cache := func(objs ...interface{}) *KubernetesCache {
		cache := KubernetesCache{
			FieldLogger: fixture.NewTestLogger(t),
		}
		for _, o := range objs {
			cache.Insert(o)
		}
		return &cache
	}

	service := func(namespace, name string) *v1.Service {
		return &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
	}

	ingressBackendService := func(namespace, name string) *networking_v1.Ingress {
		return &networking_v1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: networking_v1.IngressSpec{
				Rules: []networking_v1.IngressRule{{
					Host: "test.projectcontour.io",
					IngressRuleValue: networking_v1.IngressRuleValue{
						HTTP: &networking_v1.HTTPIngressRuleValue{
							Paths: []networking_v1.HTTPIngressPath{{
								Backend: networking_v1.IngressBackend{
									Service: &networking_v1.IngressServiceBackend{
										Name: name,
										Port: networking_v1.ServiceBackendPort{
											Number: 80,
										},
									},
								},
							}},
						},
					},
				}},
			},
		}
	}

	ingressDefaultBackend := func(namespace, name string) *networking_v1.Ingress {
		return &networking_v1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: networking_v1.IngressSpec{
				DefaultBackend: backendv1(name, intstr.FromInt(80)),
				Rules: []networking_v1.IngressRule{{
					Host: "test.projectcontour.io",
				}},
			},
		}
	}

	httpProxy := func(namespace, name string) *contour_api_v1.HTTPProxy {
		return &contour_api_v1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: contour_api_v1.HTTPProxySpec{
				Routes: []contour_api_v1.Route{{
					Services: []contour_api_v1.Service{{
						Name: name,
						Port: 80,
					}},
				}},
				TCPProxy: nil,
				Includes: nil,
			},
		}
	}

	tcpProxy := func(namespace, name string) *contour_api_v1.HTTPProxy {
		return &contour_api_v1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: contour_api_v1.HTTPProxySpec{
				TCPProxy: &contour_api_v1.TCPProxy{
					Services: []contour_api_v1.Service{{
						Name: name,
						Port: 90,
					}},
				},
				Includes: nil,
			},
		}
	}

	httpRoute := func(namespace, name string) *gatewayapi_v1alpha1.HTTPRoute {
		return &gatewayapi_v1alpha1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
				Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
					ForwardTo: []gatewayapi_v1alpha1.HTTPRouteForwardTo{{
						ServiceName: pointer.StringPtr(name),
					}},
				}},
			},
		}
	}

	tests := map[string]struct {
		cache *KubernetesCache
		svc   *v1.Service
		want  bool
	}{
		"empty cache does not trigger rebuild": {
			cache: cache(),
			svc:   service("default", "service-1"),
			want:  false,
		},
		"ingress backend exists in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				ingressBackendService("default", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: true,
		},
		"ingress backend does not exist in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				ingressBackendService("user", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: false,
		},
		"ingress default backend exists in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				ingressDefaultBackend("default", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: true,
		},
		"ingress default backend does not exist in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				ingressDefaultBackend("user", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: false,
		},
		"httpproxy exists in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				httpProxy("default", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: true,
		},
		"httpproxy does not exist in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				httpProxy("user", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: false,
		},
		"tcproxy exists in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				tcpProxy("default", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: true,
		},
		"tcpproxy does not exist in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				tcpProxy("user", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: false,
		},
		"httproute exists in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				httpRoute("default", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: true,
		},
		"httproute does not exist in same namespace as service": {
			cache: cache(
				service("default", "service-1"),
				httpRoute("user", "service-1"),
			),
			svc:  service("default", "service-1"),
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.cache.serviceTriggersRebuild(tc.svc))
		})
	}
}

func TestSecretTriggersRebuild(t *testing.T) {

	secret := func(namespace, name string) *v1.Secret {
		return &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Type: v1.SecretTypeTLS,
			Data: secretdata(fixture.CERTIFICATE, fixture.RSA_PRIVATE_KEY),
		}
	}

	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca",
			Namespace: "default",
		},
		Data: map[string][]byte{
			CACertificateKey: []byte(fixture.CERTIFICATE),
		},
	}

	tlsCertificateDelegation := func(namespace, name string, targetNamespaces ...string) *contour_api_v1.TLSCertificateDelegation {
		return &contour_api_v1.TLSCertificateDelegation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: contour_api_v1.TLSCertificateDelegationSpec{
				Delegations: []contour_api_v1.CertificateDelegation{{
					SecretName:       name,
					TargetNamespaces: targetNamespaces,
				}},
			},
		}
	}

	ingress := func(namespace, name, secretName string) *networking_v1.Ingress {
		return &networking_v1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: networking_v1.IngressSpec{
				TLS: []networking_v1.IngressTLS{{
					SecretName: secretName,
				}},
			},
		}
	}

	cache := func(objs ...interface{}) *KubernetesCache {
		cache := KubernetesCache{
			FieldLogger: fixture.NewTestLogger(t),
		}
		for _, o := range objs {
			cache.Insert(o)
		}
		return &cache
	}

	httpProxy := func(namespace, name, secretName string) *contour_api_v1.HTTPProxy {
		return &contour_api_v1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: contour_api_v1.HTTPProxySpec{
				VirtualHost: &contour_api_v1.VirtualHost{
					Fqdn: "",
					TLS: &contour_api_v1.TLS{
						SecretName: secretName,
					},
				},
			},
		}
	}

	tests := map[string]struct {
		cache  *KubernetesCache
		secret *v1.Secret
		want   bool
	}{
		"empty cache does not trigger rebuild": {
			cache:  cache(),
			secret: secret("default", "secret"),
			want:   false,
		},
		"CA secret triggers rebuild": {
			cache:  cache(),
			secret: caSecret,
			want:   true,
		},
		"ingress secret triggers rebuild": {
			cache: cache(
				ingress("default", "secret", "secret"),
			),
			secret: secret("default", "secret"),
			want:   true,
		},
		"ingress with delegated secret (specific namespace) triggers rebuild": {
			cache: cache(
				tlsCertificateDelegation("default", "tlscert", "user"),
				ingress("user", "ingress", "default/tlscert"),
			),
			secret: secret("default", "tlscert"),
			want:   true,
		},
		"ingress with delegated secret ('*' namespace) triggers rebuild": {
			cache: cache(
				tlsCertificateDelegation("default", "tlscert", "*"),
				ingress("user", "ingress", "default/tlscert"),
			),
			secret: secret("default", "tlscert"),
			want:   true,
		},
		"httpproxy empty vhost does not trigger rebuild": {
			cache: cache(
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "proxy",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{},
				},
			),
			secret: secret("default", "tlscert"),
			want:   false,
		},
		"httpproxy empty TLS does not trigger rebuild": {
			cache: cache(
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "proxy",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "test.projectcontour.io",
						},
					},
				},
			),
			secret: secret("default", "tlscert"),
			want:   false,
		},
		"httpproxy secret triggers rebuild": {
			cache: cache(
				httpProxy("default", "proxy", "tlscert"),
			),
			secret: secret("default", "tlscert"),
			want:   true,
		},
		"httpproxy with delegated secret (specific namespace) triggers rebuild": {
			cache: cache(
				tlsCertificateDelegation("default", "tlscert", "user"),
				httpProxy("user", "ingress", "default/tlscert"),
			),
			secret: secret("default", "tlscert"),
			want:   true,
		},
		"httpproxy with delegated secret ('*' namespace) triggers rebuild": {
			cache: cache(
				tlsCertificateDelegation("default", "tlscert", "*"),
				httpProxy("user", "ingress", "default/tlscert"),
			),
			secret: secret("default", "tlscert"),
			want:   true,
		},
		"configuration file secret triggers rebuild": {
			cache: &KubernetesCache{
				FieldLogger: fixture.NewTestLogger(t),
				ConfiguredSecretRefs: []*types.NamespacedName{{
					Namespace: "user",
					Name:      "tlscert",
				}},
			},
			secret: secret("user", "tlscert"),
			want:   true,
		},
		"no defined gateway does not trigger rebuild": {
			cache: &KubernetesCache{
				FieldLogger: fixture.NewTestLogger(t),
				gateway:     nil,
			},
			secret: secret("default", "tlscert"),
			want:   false,
		},
		"gateway does not define TLS on listener, does not trigger rebuild": {
			cache: cache(
				&gatewayapi_v1alpha1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						Listeners: []gatewayapi_v1alpha1.Listener{{
							TLS: nil,
						}},
					},
				},
			),
			secret: secret("default", "tlscert"),
			want:   false,
		},
		"gateway does not define TLS.CertificateRef on listener, does not trigger rebuild": {
			cache: cache(
				&gatewayapi_v1alpha1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						Listeners: []gatewayapi_v1alpha1.Listener{{
							TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
								CertificateRef: nil,
							},
						}},
					},
				},
			),
			secret: secret("default", "tlscert"),
			want:   false,
		},
		"gateway listener references secret, triggers rebuild (core Group)": {
			cache: cache(
				&gatewayapi_v1alpha1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						Listeners: []gatewayapi_v1alpha1.Listener{{
							TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
								CertificateRef: &gatewayapi_v1alpha1.LocalObjectReference{
									Group: "core",
									Kind:  "Secret",
									Name:  "tlscert",
								},
							},
						}},
					},
				},
			),
			secret: secret("projectcontour", "tlscert"),
			want:   true,
		},
		"gateway listener references secret, triggers rebuild (v1 Group)": {
			cache: cache(
				&gatewayapi_v1alpha1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "contour",
						Namespace: "projectcontour",
					},
					Spec: gatewayapi_v1alpha1.GatewaySpec{
						Listeners: []gatewayapi_v1alpha1.Listener{{
							TLS: &gatewayapi_v1alpha1.GatewayTLSConfig{
								CertificateRef: &gatewayapi_v1alpha1.LocalObjectReference{
									Group: "core",
									Kind:  "Secret",
									Name:  "tlscert",
								},
							},
						}},
					},
				},
			),
			secret: secret("projectcontour", "tlscert"),
			want:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.cache.secretTriggersRebuild(tc.secret))
		})
	}
}
