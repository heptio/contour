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
	"fmt"
	"testing"
	"time"

	"k8s.io/utils/pointer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestComputeHosts(t *testing.T) {
	tests := map[string]struct {
		route     *gatewayapi_v1alpha1.HTTPRoute
		want      []string
		wantError []error
	}{
		"single host": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"test.projectcontour.io",
					},
				},
			},
			want:      []string{"test.projectcontour.io"},
			wantError: []error(nil),
		},
		"single DNS label hostname": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"projectcontour",
					},
				},
			},
			want:      []string{"projectcontour"},
			wantError: []error(nil),
		},
		"multiple hosts": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"test.projectcontour.io",
						"test1.projectcontour.io",
						"test2.projectcontour.io",
						"test3.projectcontour.io",
					},
				},
			},
			want:      []string{"test.projectcontour.io", "test1.projectcontour.io", "test2.projectcontour.io", "test3.projectcontour.io"},
			wantError: []error(nil),
		},
		"no host": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{},
			},
			want:      []string{"*"},
			wantError: []error(nil),
		},
		"IP in host": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"1.2.3.4",
					},
				},
			},
			want: []string(nil),
			wantError: []error{
				fmt.Errorf("hostname \"1.2.3.4\" must be a DNS name, not an IP address"),
			},
		},
		"valid wildcard hostname": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"*.projectcontour.io",
					},
				},
			},
			want:      []string{"*.projectcontour.io"},
			wantError: []error(nil),
		},
		"invalid wildcard hostname": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"*.*.projectcontour.io",
					},
				},
			},
			want: []string(nil),
			wantError: []error{
				fmt.Errorf("invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"invalid hostname": {
			route: &gatewayapi_v1alpha1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"#projectcontour.io",
					},
				},
			},
			want: []string(nil),
			wantError: []error{
				fmt.Errorf("invalid listener hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			}

			got, gotError := processor.computeHosts(tc.route)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantError, gotError)
		})
	}
}

func TestMatchesConflict(t *testing.T) {
	tests := map[string]struct {
		routes []*gatewayapi_v1alpha1.HTTPRoute
		want   []*gatewayapi_v1alpha1.HTTPRoute
	}{
		"no conflict, simple": {
			routes: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/a"),
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}}, 2021),
				httpRoute("basic2",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/b"),
						ForwardTo: httpRouteForwardTo("blogsvc2", 80, 1),
					}}, 2021),
				httpRoute("basic3",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/c"),
						ForwardTo: httpRouteForwardTo("blogsvc3", 80, 1),
					}}, 2021),
				httpRoute("basic4",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/d"),
						ForwardTo: httpRouteForwardTo("blogsvc4", 80, 1),
					}}, 2021),
				httpRoute("basic5",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/e"),
						ForwardTo: httpRouteForwardTo("blogsvc5", 80, 1),
					}}, 2021),
				httpRoute("basic6",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/f"),
						ForwardTo: httpRouteForwardTo("blogsvc6", 80, 1),
					}}, 2021),
			},
			want: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/a"),
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}}, 2021),
				httpRoute("basic2",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/b"),
						ForwardTo: httpRouteForwardTo("blogsvc2", 80, 1),
					}}, 2021),
				httpRoute("basic3",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/c"),
						ForwardTo: httpRouteForwardTo("blogsvc3", 80, 1),
					}}, 2021),
				httpRoute("basic4",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/d"),
						ForwardTo: httpRouteForwardTo("blogsvc4", 80, 1),
					}}, 2021),
				httpRoute("basic5",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/e"),
						ForwardTo: httpRouteForwardTo("blogsvc5", 80, 1),
					}}, 2021),
				httpRoute("basic6",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/f"),
						ForwardTo: httpRouteForwardTo("blogsvc6", 80, 1),
					}}, 2021),
			},
		},
		"no conflict, same path, different headers": {
			routes: []*gatewayapi_v1alpha1.HTTPRoute{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
							Path: &gatewayapi_v1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
								Value: pointer.StringPtr("/"),
							},
							Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
								Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
								Values: map[string]string{"something": "different"},
							},
						}},
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
							Path: &gatewayapi_v1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
								Value: pointer.StringPtr("/"),
							},
							Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
								Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
								Values: map[string]string{"foo": "bar"},
							},
						}},
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}},
				},
			}},
			want: []*gatewayapi_v1alpha1.HTTPRoute{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
							Path: &gatewayapi_v1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
								Value: pointer.StringPtr("/"),
							},
							Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
								Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
								Values: map[string]string{"something": "different"},
							},
						}},
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic",
					Namespace: "projectcontour",
					Labels: map[string]string{
						"app":  "contour",
						"type": "controller",
					},
				},
				Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
					Hostnames: []gatewayapi_v1alpha1.Hostname{
						"test.projectcontour.io",
					},
					Rules: []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
							Path: &gatewayapi_v1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
								Value: pointer.StringPtr("/"),
							},
							Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
								Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
								Values: map[string]string{"foo": "bar"},
							},
						}},
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}},
				},
			}},
		},
		"no conflict, same headers, different path": {
			routes: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
							Path: &gatewayapi_v1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
								Value: pointer.StringPtr("/blog"),
							},
							Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
								Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
								Values: map[string]string{"foo": "bar"},
							},
						}},
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}}, 2021),
				httpRoute("basic2",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
							Path: &gatewayapi_v1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
								Value: pointer.StringPtr("/"),
							},
							Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
								Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
								Values: map[string]string{"foo": "bar"},
							},
						}},
						ForwardTo: httpRouteForwardTo("defaultsvc", 80, 1),
					}}, 2021),
			},
			want: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
							Path: &gatewayapi_v1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
								Value: pointer.StringPtr("/blog"),
							},
							Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
								Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
								Values: map[string]string{"foo": "bar"},
							},
						}},
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}}, 2021),
				httpRoute("basic2",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches: []gatewayapi_v1alpha1.HTTPRouteMatch{{
							Path: &gatewayapi_v1alpha1.HTTPPathMatch{
								Type:  pathMatchTypePtr(gatewayapi_v1alpha1.PathMatchPrefix),
								Value: pointer.StringPtr("/"),
							},
							Headers: &gatewayapi_v1alpha1.HTTPHeaderMatch{
								Type:   headerMatchTypePtr(gatewayapi_v1alpha1.HeaderMatchExact),
								Values: map[string]string{"foo": "bar"},
							},
						}},
						ForwardTo: httpRouteForwardTo("defaultsvc", 80, 1),
					}}, 2021),
			},
		},
		"no conflict, multiple hostnames, same matches": {
			routes: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}}, 2021),
				httpRoute("basic2",
					[]gatewayapi_v1alpha1.Hostname{"test2.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}}, 2021),
			},
			want: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}}, 2021),
				httpRoute("basic2",
					[]gatewayapi_v1alpha1.Hostname{"test2.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/"),
						ForwardTo: httpRouteForwardTo("blogsvc", 80, 1),
					}}, 2021),
			},
		},
		"duplicate prefixMatches": {
			routes: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
						ForwardTo: httpRouteForwardTo("bluesvc", 80, 1),
					}}, 2021),
				httpRoute("basic3",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
						ForwardTo: httpRouteForwardTo("purplesvc", 80, 1),
					}}, 1982),
				httpRoute("basic2",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
						ForwardTo: httpRouteForwardTo("greensvc", 80, 1),
					}}, 2004),
			},
			want: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic3",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
						ForwardTo: httpRouteForwardTo("purplesvc", 80, 1),
					}}, 1982),
			},
		},
		"same path, different match types": {
			routes: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchExact, "/blog"),
						ForwardTo: httpRouteForwardTo("bluesvc", 80, 1),
					}}, 2021),
				httpRoute("basic2",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
						ForwardTo: httpRouteForwardTo("greensvc", 80, 1),
					}}, 1982),
			},
			want: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchExact, "/blog"),
						ForwardTo: httpRouteForwardTo("bluesvc", 80, 1),
					}}, 2021),
				httpRoute("basic2",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
						ForwardTo: httpRouteForwardTo("greensvc", 80, 1),
					}}, 1982),
			},
		},
		"same prefixMatch, same match types, same timestamps, valid route is chosen alphabetically": {
			routes: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
						ForwardTo: httpRouteForwardTo("bluesvc", 80, 1),
					}}, 2021),
				httpRoute("different",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
						ForwardTo: httpRouteForwardTo("greensvc", 80, 1),
					}}, 2021),
			},
			want: []*gatewayapi_v1alpha1.HTTPRoute{
				httpRoute("basic",
					[]gatewayapi_v1alpha1.Hostname{"test.projectcontour.io"}, []gatewayapi_v1alpha1.HTTPRouteRule{{
						Matches:   httpRouteMatch(gatewayapi_v1alpha1.PathMatchPrefix, "/blog"),
						ForwardTo: httpRouteForwardTo("bluesvc", 80, 1),
					}}, 2021),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			gotValid, _ := filterConflictingRoutes(tc.routes)
			assert.Equal(t, tc.want, gotValid)
		})
	}
}

func httpRoute(name string, hostnames []gatewayapi_v1alpha1.Hostname, rules []gatewayapi_v1alpha1.HTTPRouteRule, creationYear int) *gatewayapi_v1alpha1.HTTPRoute {
	route := &gatewayapi_v1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "projectcontour",
			Labels: map[string]string{
				"app":  "contour",
				"type": "controller",
			},
			CreationTimestamp: metav1.NewTime(time.Date(creationYear, time.Month(2), 21, 1, 10, 30, 0, time.UTC)),
		},
		Spec: gatewayapi_v1alpha1.HTTPRouteSpec{
			Hostnames: hostnames,
			Rules:     rules,
		},
	}
	return route
}
