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

package annotation

import (
	"fmt"
	"testing"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestParseUint32(t *testing.T) {
	tests := map[string]struct {
		s    string
		want uint32
	}{
		"blank": {
			s:    "",
			want: 0,
		},
		"negative": {
			s:    "-6", // for alice
			want: 0,
		},
		"explicit": {
			s:    "0",
			want: 0,
		},
		"positive": {
			s:    "2",
			want: 2,
		},
		"too large": {
			s:    "144115188075855872", // larger than uint32
			want: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseUInt32(tc.s)
			if got != tc.want {
				t.Fatalf("expected: %v, got %v", tc.want, got)
			}
		})
	}
}

func TestParseUpstreamProtocols(t *testing.T) {
	tests := map[string]struct {
		a    map[string]string
		want map[string]string
	}{
		"nada": {
			a:    nil,
			want: map[string]string{},
		},
		"empty": {
			a:    map[string]string{"projectcontour.io/upstream-protocol.h2": ""},
			want: map[string]string{},
		},
		"empty with spaces": {
			a:    map[string]string{"projectcontour.io/upstream-protocol.h2": ", ,"},
			want: map[string]string{},
		},
		"single value": {
			a: map[string]string{"projectcontour.io/upstream-protocol.h2": "80"},
			want: map[string]string{
				"80": "h2",
			},
		},
		"tls": {
			a: map[string]string{"projectcontour.io/upstream-protocol.tls": "https,80"},
			want: map[string]string{
				"80":    "tls",
				"https": "tls",
			},
		},
		"multiple value": {
			a: map[string]string{"projectcontour.io/upstream-protocol.h2": "80,http,443,https"},
			want: map[string]string{
				"80":    "h2",
				"http":  "h2",
				"443":   "h2",
				"https": "h2",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ParseUpstreamProtocols(tc.a)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWebsocketRoutes(t *testing.T) {
	tests := map[string]struct {
		a    *networking_v1.Ingress
		want map[string]bool
	}{
		"empty": {
			a: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": "",
					},
				},
			},
			want: map[string]bool{},
		},
		"empty with spaces": {
			a: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": ", ,",
					},
				},
			},
			want: map[string]bool{},
		},
		"single value": {
			a: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": "/ws1",
					},
				},
			},
			want: map[string]bool{
				"/ws1": true,
			},
		},
		"multiple values": {
			a: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": "/ws1,/ws2",
					},
				},
			},
			want: map[string]bool{
				"/ws1": true,
				"/ws2": true,
			},
		},
		"multiple values with spaces and invalid entries": {
			a: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": " /ws1, , /ws2 ",
					},
				},
			},
			want: map[string]bool{
				"/ws1": true,
				"/ws2": true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := WebsocketRoutes(tc.a)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHttpAllowed(t *testing.T) {
	tests := map[string]struct {
		i     *networking_v1.Ingress
		valid bool
	}{
		"basic ingress": {
			i: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: networking_v1.IngressSpec{
					TLS: []networking_v1.IngressTLS{{
						Hosts:      []string{"whatever.example.com"},
						SecretName: "secret",
					}},
					DefaultBackend: backend("backend", intstr.FromInt(80)),
				},
			},
			valid: true,
		},
		"kubernetes.io/ingress.allow-http: \"false\"": {
			i: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.allow-http": "false",
					},
				},
				Spec: networking_v1.IngressSpec{
					TLS: []networking_v1.IngressTLS{{
						Hosts:      []string{"whatever.example.com"},
						SecretName: "secret",
					}},
					DefaultBackend: backend("backend", intstr.FromInt(80)),
				},
			},
			valid: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := HTTPAllowed(tc.i)
			want := tc.valid
			if got != want {
				t.Fatalf("got: %v, want: %v", got, want)
			}
		})
	}
}

func TestAnnotationCompat(t *testing.T) {
	tests := map[string]struct {
		svc   *core_v1.Service
		value string
	}{
		"no annotations": {
			value: "",
			svc: &core_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
		},
		"projectcontour.io/annotation": {
			value: "200",
			svc: &core_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/annotation": "200",
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ContourAnnotation(tc.svc, "annotation")
			want := tc.value
			if got != want {
				t.Fatalf("got: %v, want: %v", got, want)
			}
		})
	}
}

func TestAnnotationKindValidation(t *testing.T) {
	type status struct {
		known bool
		valid bool
	}
	tests := map[string]struct {
		obj         meta_v1.Object
		annotations map[string]status
	}{
		"service": {
			obj: &core_v1.Service{},
			annotations: map[string]status{
				"foo.invalid.com/annotation": {
					known: false, valid: false,
				},
				"projectcontour.io/annotation": {
					known: true, valid: false,
				},
			},
		},
		"httpproxy": {
			obj: &contour_api_v1.HTTPProxy{},
			annotations: map[string]status{
				// Valid only on Service.
				"projectcontour.io/max-requests": {
					known: true, valid: false,
				},
				// Valid only on Ingress.
				"projectcontour.io/websocket-routes": {
					known: true, valid: false,
				},
			},
		},
		"secrets": {
			obj: &core_v1.Secret{},
			annotations: map[string]status{
				// In our namespace but not valid on this kind.
				"projectcontour.io/ingress.class": {
					known: true, valid: false,
				},
				// Unknown, so potentially valid.
				"foo.io/secret-sauce": {
					known: false, valid: true,
				},
			},
		},
	}

	// Trivially check that everything specified in the global
	// table is valid.
	for _, kind := range []string{
		kindOf(&core_v1.Service{}),
		kindOf(&networking_v1.Ingress{}),
		kindOf(&contour_api_v1.HTTPProxy{}),
	} {
		for key := range annotationsByKind[kind] {
			t.Run(fmt.Sprintf("%s is known and valid for %s", key, kind),
				func(t *testing.T) {
					assert.Equal(t, true, IsKnown(key))
					assert.Equal(t, true, ValidForKind(kind, key))
				})
		}
	}

	// Check corner case combinations for different types.
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for k, s := range tc.annotations {
				assert.Equal(t, s.known, IsKnown(k))
				assert.Equal(t, s.valid, ValidForKind(kindOf(tc.obj), k))
			}
		})
	}
}

func TestMatchIngressClass(t *testing.T) {

	// This is a matrix test, we are testing the annotation parser
	// across various annotations, with two options:
	// ingress class is empty
	// ingress class is not empty.
	tests := map[string]struct {
		fixture meta_v1.Object
		// these are results for empty and "contour" ingress class
		// respectively.
		want []bool
	}{
		"ingress nginx kubernetes.io/ingress.class": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			want: []bool{false, false},
		},
		"ingress nginx projectcontour.io/ingress.class": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": "nginx",
					},
				},
			},
			want: []bool{false, false},
		},
		"ingress contour kubernetes.io/ingress.class": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": DEFAULT_INGRESS_CLASS_NAME,
					},
				},
			},
			want: []bool{true, true},
		},
		"ingress contour projectcontour.io/ingress.class": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "incorrect",
					Namespace: "default",
					Annotations: map[string]string{
						"projectcontour.io/ingress.class": DEFAULT_INGRESS_CLASS_NAME,
					},
				},
			},
			want: []bool{true, true},
		},
		"no annotation": {
			fixture: &networking_v1.Ingress{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "noannotation",
					Namespace: "default",
				},
			},
			want: []bool{true, false},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cases := []string{"", DEFAULT_INGRESS_CLASS_NAME}
			for i := 0; i < len(cases); i++ {
				got := MatchesIngressClass(tc.fixture, cases[i])
				if tc.want[i] != got {
					t.Errorf("matching %v against ingress class %q: expected %v, got %v", tc.fixture, cases[i], tc.want[i], got)
				}
			}

		})
	}
}

func backend(name string, port intstr.IntOrString) *networking_v1.IngressBackend {
	var portObj networking_v1.ServiceBackendPort
	if port.Type == intstr.Int {
		portObj = networking_v1.ServiceBackendPort{
			Number: port.IntVal,
		}
	} else {
		portObj = networking_v1.ServiceBackendPort{
			Name: port.StrVal,
		}
	}

	return &networking_v1.IngressBackend{
		Service: &networking_v1.IngressServiceBackend{
			Name: name,
			Port: portObj,
		},
	}
}

// kindOf returns the kind string for the given Kubernetes object.
//
// The API machinery doesn't populate the metav1.TypeMeta field for
// objects, so we have to use a type assertion to detect kinds that
// we care about.
// TODO(youngnick): This is a straight copy from internal/k8s/kind.go
// Needs to be moved to a separate module somewhere.
func kindOf(obj interface{}) string {
	switch obj.(type) {
	case *core_v1.Secret:
		return "Secret"
	case *core_v1.Service:
		return "Service"
	case *networking_v1.Ingress:
		return "Ingress"
	case *contour_api_v1.HTTPProxy:
		return "HTTPProxy"
	case *contour_api_v1.TLSCertificateDelegation:
		return "TLSCertificateDelegation"
	default:
		return ""
	}
}
