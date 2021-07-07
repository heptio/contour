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

package v3

import (
	"testing"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/golang/protobuf/proto"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/protobuf"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestSecretCacheContents(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_tls_v3.Secret
		want     []proto.Message
	}{
		"empty": {
			contents: nil,
			want:     nil,
		},
		"simple": {
			contents: secretmap(
				secret("default/secret/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
			want: []proto.Message{
				secret("default/secret/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var sc SecretCache
			sc.Update(tc.contents)
			got := sc.Contents()
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestSecretCacheQuery(t *testing.T) {
	tests := map[string]struct {
		contents map[string]*envoy_tls_v3.Secret
		query    []string
		want     []proto.Message
	}{
		"exact match": {
			contents: secretmap(
				secret("default/secret/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
			query: []string{"default/secret/68621186db"},
			want: []proto.Message{
				secret("default/secret/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
		},
		"partial match": {
			contents: secretmap(
				secret("default/secret-a/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				secret("default/secret-b/5397c67313", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			),
			query: []string{"default/secret/68621186db", "default/secret-b/5397c67313"},
			want: []proto.Message{
				secret("default/secret-b/5397c67313", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			},
		},
		"no match": {
			contents: secretmap(
				secret("default/secret/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
			query: []string{"default/secret-b/5397c67313"},
			want:  nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var sc SecretCache
			sc.Update(tc.contents)
			got := sc.Query(tc.query)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestSecretVisit(t *testing.T) {
	tests := map[string]struct {
		objs []interface{}
		want map[string]*envoy_tls_v3.Secret
	}{
		"nothing": {
			objs: nil,
			want: map[string]*envoy_tls_v3.Secret{},
		},
		"unassociated secrets": {
			objs: []interface{}{
				tlssecret("default", "secret-a", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				tlssecret("default", "secret-b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			},
			want: map[string]*envoy_tls_v3.Secret{},
		},
		"simple ingress with secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple ingresses with shared secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"omg.example.com"},
							SecretName: "secret",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "omg.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 8080),
									}},
								},
							},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple ingresses with different secrets": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kuard",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"whatever.example.com"},
							SecretName: "secret-a",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "whatever.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 80),
									}},
								},
							},
						}},
					},
				},
				&networking_v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: networking_v1.IngressSpec{
						TLS: []networking_v1.IngressTLS{{
							Hosts:      []string{"omg.example.com"},
							SecretName: "secret-b",
						}},
						Rules: []networking_v1.IngressRule{{
							Host: "omg.example.com",
							IngressRuleValue: networking_v1.IngressRuleValue{
								HTTP: &networking_v1.HTTPIngressRuleValue{
									Paths: []networking_v1.HTTPIngressPath{{
										Backend: *backend("kuard", 80),
									}},
								},
							},
						}},
					},
				},
				tlssecret("default", "secret-a", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				tlssecret("default", "secret-b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret-a/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				secret("default/secret-b/5397c67313", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY)),
			),
		},
		"simple httpproxy with secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple httpproxies with shared secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www1.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www2.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				tlssecret("default", "secret", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			},
			want: secretmap(
				secret("default/secret/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
			),
		},
		"multiple httpproxies with different secret": {
			objs: []interface{}{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "backend",
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-a",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www1.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret-a",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-b",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www2.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret-b",
							},
						},
						Routes: []contour_api_v1.Route{{
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				tlssecret("default", "secret-a", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				tlssecret("default", "secret-b", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			},
			want: secretmap(
				secret("default/secret-a/68621186db", secretdata(CERTIFICATE, RSA_PRIVATE_KEY)),
				secret("default/secret-b/5397c67313", secretdata(CERTIFICATE_2, RSA_PRIVATE_KEY_2)),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := buildDAG(t, tc.objs...)
			got := visitSecrets(root)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

// buildDAG produces a dag.DAG from the supplied objects.
func buildDAG(t *testing.T, objs ...interface{}) *dag.DAG {
	builder := dag.Builder{
		Source: dag.KubernetesCache{
			FieldLogger: fixture.NewTestLogger(t),
		},
		Processors: []dag.Processor{
			&dag.IngressProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			},
			&dag.HTTPProxyProcessor{},
			&dag.ListenerProcessor{},
		},
	}

	for _, o := range objs {
		builder.Source.Insert(o)
	}
	return builder.Build()
}

// buildDAGFallback produces a dag.DAG from the supplied objects with a fallback cert configured.
func buildDAGFallback(t *testing.T, fallbackCertificate *types.NamespacedName, objs ...interface{}) *dag.DAG {
	builder := dag.Builder{
		Source: dag.KubernetesCache{
			FieldLogger: fixture.NewTestLogger(t),
		},
		Processors: []dag.Processor{
			&dag.IngressProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			},
			&dag.HTTPProxyProcessor{
				FallbackCertificate: fallbackCertificate,
			},
			&dag.ListenerProcessor{},
		},
	}
	for _, o := range objs {
		builder.Source.Insert(o)
	}
	return builder.Build()
}

func secretmap(secrets ...*envoy_tls_v3.Secret) map[string]*envoy_tls_v3.Secret {
	m := make(map[string]*envoy_tls_v3.Secret)
	for _, s := range secrets {
		m[s.Name] = s
	}
	return m
}

func secret(name string, data map[string][]byte) *envoy_tls_v3.Secret {
	return &envoy_tls_v3.Secret{
		Name: name,
		Type: &envoy_tls_v3.Secret_TlsCertificate{
			TlsCertificate: &envoy_tls_v3.TlsCertificate{
				CertificateChain: &envoy_core_v3.DataSource{
					Specifier: &envoy_core_v3.DataSource_InlineBytes{
						InlineBytes: data[v1.TLSCertKey],
					},
				},
				PrivateKey: &envoy_core_v3.DataSource{
					Specifier: &envoy_core_v3.DataSource_InlineBytes{
						InlineBytes: data[v1.TLSPrivateKeyKey],
					},
				},
			},
		},
	}
}

// tlssecert creates a new v1.Secret object of type kubernetes.io/tls.
func tlssecret(namespace, name string, data map[string][]byte) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: v1.SecretTypeTLS,
		Data: data,
	}
}

func backend(name string, port int32) *networking_v1.IngressBackend {
	return &networking_v1.IngressBackend{
		Service: &networking_v1.IngressServiceBackend{
			Name: name,
			Port: networking_v1.ServiceBackendPort{Number: port},
		},
	}
}

// nolint:revive
const (
	// generated by https://www.selfsignedcertificate.com
	CERTIFICATE = `-----BEGIN CERTIFICATE-----
MIIDHTCCAgWgAwIBAgIJAOv27DGlF3qdMA0GCSqGSIb3DQEBBQUAMCUxIzAhBgNV
BAMMGmJvcmluZy13b3puaWFrLmV4YW1wbGUuY29tMB4XDTE5MTIwNTAxMzQzM1oX
DTI5MTIwMjAxMzQzM1owJTEjMCEGA1UEAwwaYm9yaW5nLXdvem5pYWsuZXhhbXBs
ZS5jb20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDbgwFwfbikZxPb
NYidPuNJoexq5W9fJrB/3jqsWox8pfess0bw/EL/VcEUqlrcuo40Md0MxApPuoPj
eZCOZYhrA2XgcVTMnq61vusnuvmeG/qcrd5apSOoopSo2pmmI1rsJ1AVpheA+eR6
uoWVILK8uYtPmcOQAoCU/E6iZYDLZ0AEiU16kz/cGfWx9lBukd+LQ+ZRQnLDiEI/
4hRmrZrEdJoDglzIgJVI+c8OfwbLq5eRMY2fYnxqm/1BJhqjDBc4Q8ufYgfOwobu
JdVoSgiFy7wyH0GxMk4LRR6yJXLs1yjaihLERbjzlStvFVl4yidpE6Bi0amKW8HT
Qxgk7iRRAgMBAAGjUDBOMB0GA1UdDgQWBBTLcIMeWLFiL2waFL6FPomNZR7gFDAf
BgNVHSMEGDAWgBTLcIMeWLFiL2waFL6FPomNZR7gFDAMBgNVHRMEBTADAQH/MA0G
CSqGSIb3DQEBBQUAA4IBAQBQLWokaWuFeSWLpxxaBX6aatgKAKNUSqDWNzM9zVMH
xJVDywWJT3pwq7JUXujVS/c9mzCPJEsn7OQPihQECRq09l/nBK0kn9I1X6X1SMtD
OJbpEWfQQxgstdgeC6pxrZRanF5a7EWO0pFSfjuM1ABjsdExaG3C8+wgEqOjHFDS
NaW826GOFf/uMOnavpG6QePECAtJVpLAZPw6Rah6cAZrYUUezM/Tg+8JUhYUS20F
STZG5knGQIe6kksWGkJUhMu8xLdH2HKtUVAkDu7jITy2WZbg0O/Pxe30b4qyt29Y
813p8G+7188EFDBGNihYYVJ+GJ/d/WPoptSHJOfShtbk
-----END CERTIFICATE-----`

	RSA_PRIVATE_KEY = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA24MBcH24pGcT2zWInT7jSaHsauVvXyawf946rFqMfKX3rLNG
8PxC/1XBFKpa3LqONDHdDMQKT7qD43mQjmWIawNl4HFUzJ6utb7rJ7r5nhv6nK3e
WqUjqKKUqNqZpiNa7CdQFaYXgPnkerqFlSCyvLmLT5nDkAKAlPxOomWAy2dABIlN
epM/3Bn1sfZQbpHfi0PmUUJyw4hCP+IUZq2axHSaA4JcyICVSPnPDn8Gy6uXkTGN
n2J8apv9QSYaowwXOEPLn2IHzsKG7iXVaEoIhcu8Mh9BsTJOC0UesiVy7Nco2ooS
xEW485UrbxVZeMonaROgYtGpilvB00MYJO4kUQIDAQABAoIBAF5L671gNIZjRVNg
rtwl3MuPxJizEOHGJAH5/Ch4CWuufDPzG6GALGO1eekfuUKi3V2sofHO8UMIs4lv
elrBYRXfcs80wCHadODcL/Z0SrDSAhl2U1OLJ0NU/BmBNon5HCDgTnXOUMB2GOFj
6OiEEGQkLKU4P5tIh+X4cOswQWCeoVjW0JVgni20hi3LJNTxSNYeU5VFvPKtoBLl
8nFqF3ky+bqYfS6H6qM/mO+XL0NQ2wjMteyUeDXcVGfsf7Ir21SUw3zGaeBJl55B
6BrUgfxVOKuxkw2bwxmu8HX+CxlMMMzaRt+5URFbfOaMgXzjpikrxdeFAAGeu0m4
bidUR5UCgYEA8lRGqYfowoOCrV8Ksn8nM0Z9PlnmKM5d9mQ875sm/SYLO43h+s0D
R4VWmLzaGyi0m0036lxIthDfbbGWSjmNrgQ0YIS7ilmBPMUKKYzXgDoiI76aJBTz
UMpWutb+VYimPPorLKcxNb3BjR3QHx7vCRS2gV5izV0djtMkKc53OXsCgYEA5+Uz
A7cmO8gHyxlW6SA3+wMH6VKP5ABTkDmKfRF3NCv4UHNn4TtlNuS1D3ZMNXWgCtz6
qJ/bRTAqseBIX15pzR/MvyNmHRUN3A2Ba6vB2pJux+ZyQjxn3Z+gisjX+eN3LvTU
YpcJNi0HSuV57n4AAk5YPO5iMEFw95vfBn3MMaMCgYEAnFwyqAsQ7gmLVTDBJ0GS
Wqx9/bBmKShXSreM9hIHi0pz7v5ytLB6EDkCElWw6dtPBfJCRQ88v3WNpSr0TXpr
Z8BAx5J9rBxqnnqJPxwopQ1dn/DJZsS55wRYCADXZPtiQHAvUYWj5AhHjjWRZ7M/
C3348OqlF9ugSdsFN5CIL2cCgYEAqt5lop03XOFdbLe1JH4LAbgQAkpFoDjlWeYs
N0/BR/4GMDF5H6sGP1ZyW3xNVy7eyGJfiBSSGv8M1phue2c0CmMeGNDakx9KYRTK
gi3C32z6l+0jz852sgTG5Lxs98I1tbHNNQAZV4QCVZuVJrhNBWX4+pykWO4/cRO3
WC8lYIUCgYBmmN4z0MR2YWoRvN3lYey3bRGAvsSU6ouiFo40UZdZaRXc1sA3oc+5
6Di3f8eOIhM5IekOBoaTBf90V8seB6Nw+/jzAViG1HDI7k0ZOoApDuFS6NYk1/bU
dk98FvYdyAjjgNsxXCyx7vIgYU3OgVNgvFsFubX/Uk66fcfCpPBMLg==
-----END RSA PRIVATE KEY-----`

	CERTIFICATE_2 = `-----BEGIN CERTIFICATE-----
MIIDJTCCAg2gAwIBAgIJAP5VPT9oymF8MA0GCSqGSIb3DQEBBQUAMCkxJzAlBgNV
BAMMHmV4cGxvc2l2ZS1kaWFycmhlYS5leGFtcGxlLmNvbTAeFw0xOTEyMDUwMjEw
MDlaFw0yOTEyMDIwMjEwMDlaMCkxJzAlBgNVBAMMHmV4cGxvc2l2ZS1kaWFycmhl
YS5leGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAM2V
AaUUqC1/s3b7Rr1D6YkN99dLlwgMvwE+KzPXHfupn0tMdNsSfFjeO7I+mULFwLf+
fBxzUhdn9Vx6k7/INVCl6ktPKD/nhcgW2pfideDN9sCoIzAyPOL6ysV7IkYz5UJC
FTU5zGUW9X7xcRqKyQyvXEQPc/sx8fVIDr5Aw5K64a000NTLfjcTWqEWrXqk7wdg
u7+7ggazxMGJQEyjwpDQVToQH5yIdbyjgB4rniykqGYzeixTzHeQCf7d/kYTHPrW
v9970zo6PC8Q99mFl75BR7Hdy3X9DSAcz050MwkCXSgwoYxGnWHCrI+Ynndf+RO5
YDMyXbEAT1zbD2O2ER8CAwEAAaNQME4wHQYDVR0OBBYEFBSXzvnpiVwzs9iiPABO
D0SbE8PuMB8GA1UdIwQYMBaAFBSXzvnpiVwzs9iiPABOD0SbE8PuMAwGA1UdEwQF
MAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAFQ1sHL2yqAOekYTWO4ZzDGKWbkFPjPM
hEP3ZFOUabXeR3q/n8yzb1ngFCQj/alSf6otPXZqrfdXtN4TPGhx50kf9zmX94r2
lWP+TcHyMQOJGXJQgTv1yGAgruJ638GZAiHJ0q3b3hrRVLaHwFcBaYRwtPGA9CgX
gfKxSdfTxUcuSz4gPIB3JDfRbO5E45rSZJokbjuYm5ZI5DtjSRCA6w//lDKqJGiM
6aUNFwejPFZJ1MkBgQ1muYifzRoXZ/pUwUMlEzThChe3r2nXznCbpK9LtJ6KOX0Y
dIpyQrOkc9wdjNlS2+h2IYwmJujZNoISX53P7bbAqm01guWYDASz+Wc=
-----END CERTIFICATE-----`

	RSA_PRIVATE_KEY_2 = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAzZUBpRSoLX+zdvtGvUPpiQ3310uXCAy/AT4rM9cd+6mfS0x0
2xJ8WN47sj6ZQsXAt/58HHNSF2f1XHqTv8g1UKXqS08oP+eFyBbal+J14M32wKgj
MDI84vrKxXsiRjPlQkIVNTnMZRb1fvFxGorJDK9cRA9z+zHx9UgOvkDDkrrhrTTQ
1Mt+NxNaoRateqTvB2C7v7uCBrPEwYlATKPCkNBVOhAfnIh1vKOAHiueLKSoZjN6
LFPMd5AJ/t3+RhMc+ta/33vTOjo8LxD32YWXvkFHsd3Ldf0NIBzPTnQzCQJdKDCh
jEadYcKsj5ied1/5E7lgMzJdsQBPXNsPY7YRHwIDAQABAoIBAGQNC6LivcJ+7sGO
IuxDm+mGscLG1/cj9OVO80mkfMQY1hyYVhq0EW5SoazqyD317gfdw1s5SI95mbDr
OvLQJhpc1PzXxlfrfnFVpgbbQNEqi9dRPObc3EL/GSYo+hI+eWnYrWec/HuKQ+oG
6SuotZYF1hqNhr2Onhnoerxe2o+SohNgy01oUhI+dWsjKbRYobhofxNJ3FWGCyR/
7Xh7RcatHvHlOM6JQ8rFjxB9X5WMhIhkc/O3dZLX8AKWIbfGCN0eudLUdqU+WV9X
/rDfXj7aRhKKt3bUHvcIx+H6ieCrdifYXz8bBquJxTVPctS/pI9NoIREPwC0iYPE
+NyzDcECgYEA9sy1chbhTixHFyXC+C5fT0Sj3HZq4L1be9QA1q+XjcOncPnEf75t
YmbhXP5RnAUB8RW1bnU3n7Vty6RzfXz6PC0qoPFm6xrKZ4p+yn92yI9/6a4VSObt
IlBU3VHx0kxoSk/c/H//JTGGwa1RVGLN6MV8W+t8dTYWK+MDd5wztP8CgYEA1T7x
yb9SitDFcaT6oApEye5FWSC+CkB5BNomEOtOeDkZRoBcO72x4TXRmOT7oVFU5F3x
0qmnlPpWjWiMnNBzh85fugKquwRjXFzWMSpVMPlHm7sTCNO5CKb+RKbK68qUvMzc
Y4BGCufCvvaQU/FxidtdKeRzZlVfABG+pFfbA+ECgYBBzrX3FPjAwne2SWBikuLh
HRlgWMcI5BT3wMD0fd+4clo8eq0Vru411d7zz/Bs3Lz2zuYQ7PqHAHalXVVaOa/z
yctbHONnfz5HO5uxXSmMMw9VfRC53rGOe8MVPJtxiuQoJIF1Zp/fCAS5sgBEsw/a
qIYPcIxAKMriquaqxyDWewKBgQCXIIre/haThq3HgrKUJXLm4VSIe+ny/gpGZAxC
RWFRVrYQ/vte42tjPm8SuoWSqD9PsTymndHEhT497XBp2llmT94Lx8QT0mJQnQK3
yVai5KfZOFWfFd22whLFuKdrQCD1RQKUCd6Z7/JWwAs9Uomyt6JpBBy805gGRo0j
j5gKQQKBgBTuxL1gbWJjmuRXPQrgbg7csn2UaDvKkbz3ibrSnD9BOHvm9aC2c3nY
IPjdKA1DW7i9vFcKVdwqxOnWT2/wYv2FPkV+HrxWh8KX7ZwCAWveo60g53Sr457m
s9pb3b/IYa6Tnxo6cPdhwZ3CrLlq/1IopES1SmvaS4dgMFmf/0vk
-----END RSA PRIVATE KEY-----`
)

func secretdata(cert, key string) map[string][]byte {
	return map[string][]byte{
		v1.TLSCertKey:       []byte(cert),
		v1.TLSPrivateKeyKey: []byte(key),
	}
}
