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

package e2e

import (
	"context"

	"github.com/onsi/ginkgo"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Fixtures holds references to all of the E2E fixtures helpers.
type Fixtures struct {
	// Echo provides helpers for working with the ingress-conformance-echo
	// test fixture.
	Echo *Echo

	// EchoSecure provides helpers for working with the TLS-secured
	// ingress-conformance-echo-tls test fixture.
	EchoSecure *EchoSecure

	// HTTPBin provides helpers for working with the kennethreitz/httpbin
	// test fixture.
	HTTPBin *HTTPBin
}

// Echo manages the ingress-conformance-echo fixture.
type Echo struct {
	client client.Client
	t      ginkgo.GinkgoTInterface
}

// Deploy creates the ingress-conformance-echo fixture, specifically
// the deployment and service, in the given namespace and with the given name, or
// fails the test if it encounters an error. Namespace is defaulted to "default"
// and name is defaulted to "ingress-conformance-echo" if not provided. Returns
// a cleanup function.
func (e *Echo) Deploy(ns, name string) func() {
	valOrDefault := func(val, defaultVal string) string {
		if val != "" {
			return val
		}
		return defaultVal
	}

	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "ingress-conformance-echo")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "conformance-echo",
							Image: "k8s.gcr.io/ingressconformance/echoserver@sha256:9b34b17f391f87fb2155f01da2f2f90b7a4a5c1110ed84cb5379faa4f570dc52",
							Env: []corev1.EnvVar{
								{
									Name:  "INGRESS_NAME",
									Value: name,
								},
								{
									Name:  "SERVICE_NAME",
									Value: name,
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http-api",
									ContainerPort: 3000,
								},
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(3000),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(e.t, e.client.Create(context.TODO(), deployment))

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http-api"),
				},
			},
			Selector: map[string]string{"app.kubernetes.io/name": name},
		},
	}
	require.NoError(e.t, e.client.Create(context.TODO(), service))

	return func() {
		require.NoError(e.t, e.client.Delete(context.TODO(), service))
		require.NoError(e.t, e.client.Delete(context.TODO(), deployment))
	}
}

// EchoSecure manages the TLS-secured ingress-conformance-echo fixture.
type EchoSecure struct {
	client client.Client
	t      ginkgo.GinkgoTInterface
}

// Deploy creates the TLS-secured ingress-conformance-echo-tls fixture, specifically
// the deployment and service, in the given namespace and with the given name, or
// fails the test if it encounters an error. Namespace is defaulted to "default"
// and name is defaulted to "ingress-conformance-echo-tls" if not provided. Returns
// a cleanup function.
func (e *EchoSecure) Deploy(ns, name string) func() {
	valOrDefault := func(val, defaultVal string) string {
		if val != "" {
			return val
		}
		return defaultVal
	}

	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "ingress-conformance-echo-tls")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "conformance-echo",
							Image: "k8s.gcr.io/ingressconformance/echoserver@sha256:9b34b17f391f87fb2155f01da2f2f90b7a4a5c1110ed84cb5379faa4f570dc52",
							Env: []corev1.EnvVar{
								{
									Name:  "INGRESS_NAME",
									Value: name,
								},
								{
									Name:  "SERVICE_NAME",
									Value: name,
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name:  "TLS_SERVER_CERT",
									Value: "/run/secrets/certs/tls.crt",
								},
								{
									Name:  "TLS_SERVER_PRIVKEY",
									Value: "/run/secrets/certs/tls.key",
								},
								{
									Name:  "TLS_CLIENT_CACERTS",
									Value: "/run/secrets/certs/ca.crt",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http-api",
									ContainerPort: 3000,
								},
								{
									Name:          "https-api",
									ContainerPort: 8443,
								},
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(3000),
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									MountPath: "/run/secrets/certs",
									Name:      "backend-server-cert",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "backend-server-cert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "backend-server-cert",
								},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(e.t, e.client.Create(context.TODO(), deployment))

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			Annotations: map[string]string{
				"projectcontour.io/upstream-protocol.tls": "443",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http-api"),
				},
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromString("https-api"),
				},
			},
			Selector: map[string]string{"app.kubernetes.io/name": name},
		},
	}
	require.NoError(e.t, e.client.Create(context.TODO(), service))

	return func() {
		require.NoError(e.t, e.client.Delete(context.TODO(), service))
		require.NoError(e.t, e.client.Delete(context.TODO(), deployment))
	}
}

// HTTPBin manages the kennethreitz/httpbin fixture.
type HTTPBin struct {
	client client.Client
	t      ginkgo.GinkgoTInterface
}

// Deploy creates the kennethreitz/httpbin fixture, specifically
// the deployment and service, in the given namespace and with the given name, or
// fails the test if it encounters an error. Namespace is defaulted to "default"
// and name is defaulted to "httpbin" if not provided.
func (h *HTTPBin) Deploy(ns, name string) {
	valOrDefault := func(val, defaultVal string) string {
		if val != "" {
			return val
		}
		return defaultVal
	}

	ns = valOrDefault(ns, "default")
	name = valOrDefault(name, "httpbin")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "httpbin",
							Image: "docker.io/kennethreitz/httpbin:latest",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 80,
								},
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/status/200",
										Port: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(h.t, h.client.Create(context.TODO(), deployment))

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http"),
				},
			},
			Selector: map[string]string{"app.kubernetes.io/name": name},
		},
	}
	require.NoError(h.t, h.client.Create(context.TODO(), service))
}

func IngressPathTypePtr(val networkingv1.PathType) *networkingv1.PathType {
	return &val
}
