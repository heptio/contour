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

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

func TestComputeHosts(t *testing.T) {
	tests := map[string]struct {
		listenerHost *gatewayapi_v1alpha1.Hostname
		hostnames    []gatewayapi_v1alpha1.Hostname
		want         map[string]struct{}
		wantError    []error
	}{
		"single host": {
			listenerHost: nil,
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
			},
			want: map[string]struct{}{
				"test.projectcontour.io": {},
			},
			wantError: []error(nil),
		},
		"single DNS label hostname": {
			listenerHost: nil,
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"projectcontour",
			},
			want: map[string]struct{}{
				"projectcontour": {},
			},
			wantError: []error(nil),
		},
		"multiple hosts": {
			listenerHost: nil,
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"test.projectcontour.io",
				"test1.projectcontour.io",
				"test2.projectcontour.io",
				"test3.projectcontour.io",
			},
			want: map[string]struct{}{
				"test.projectcontour.io":  {},
				"test1.projectcontour.io": {},
				"test2.projectcontour.io": {},
				"test3.projectcontour.io": {},
			},
			wantError: []error(nil),
		},
		"no host": {
			listenerHost: nil,
			hostnames:    []gatewayapi_v1alpha1.Hostname{},
			want: map[string]struct{}{
				"*": {},
			},
			wantError: []error(nil),
		},
		"IP in host": {
			listenerHost: nil,
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"1.2.3.4",
			},
			want: map[string]struct{}{},
			wantError: []error{
				fmt.Errorf("hostname \"1.2.3.4\" must be a DNS name, not an IP address"),
			},
		},
		"valid wildcard hostname": {
			listenerHost: nil,
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"*.projectcontour.io",
			},
			want: map[string]struct{}{
				"*.projectcontour.io": {},
			},
			wantError: []error(nil),
		},
		"invalid wildcard hostname": {
			listenerHost: nil,
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"*.*.projectcontour.io",
			},
			want: map[string]struct{}{},
			wantError: []error{
				fmt.Errorf("invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"invalid hostname": {
			listenerHost: nil,
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"#projectcontour.io",
			},
			want: map[string]struct{}{},
			wantError: []error{
				fmt.Errorf("invalid hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"invalid listener hostname": {
			listenerHost: listenerHostname("#projectcontour.io"),
			hostnames:    []gatewayapi_v1alpha1.Hostname{},
			want:         map[string]struct{}{},
			wantError: []error{
				fmt.Errorf("invalid hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"invalid listener wildcard hostname": {
			listenerHost: listenerHostname("*.*.projectcontour.io"),
			hostnames:    []gatewayapi_v1alpha1.Hostname{},
			want:         map[string]struct{}{},
			wantError: []error{
				fmt.Errorf("invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"listener host & hostnames host do not exactly match": {
			listenerHost: listenerHostname("listener.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"http.projectcontour.io",
			},
			want:      map[string]struct{}{},
			wantError: []error{fmt.Errorf("gateway hostname \"listener.projectcontour.io\" does not match route hostname \"http.projectcontour.io\"")},
		},
		"listener host & hostnames host exactly match": {
			listenerHost: listenerHostname("http.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"http.projectcontour.io",
			},
			want: map[string]struct{}{
				"http.projectcontour.io": {},
			},
			wantError: nil,
		},
		"listener host & multi hostnames host exactly match one host": {
			listenerHost: listenerHostname("http.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"http.projectcontour.io",
				"http2.projectcontour.io",
				"http3.projectcontour.io",
			},
			want: map[string]struct{}{
				"http.projectcontour.io": {},
			},
			wantError: []error{
				fmt.Errorf("gateway hostname \"http.projectcontour.io\" does not match route hostname \"http2.projectcontour.io\""),
				fmt.Errorf("gateway hostname \"http.projectcontour.io\" does not match route hostname \"http3.projectcontour.io\""),
			},
		},
		"listener host & hostnames host match wildcard host": {
			listenerHost: listenerHostname("*.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"http.projectcontour.io",
			},
			want: map[string]struct{}{
				"http.projectcontour.io": {},
			},
			wantError: nil,
		},
		"listener host & hostnames host do not match wildcard host": {
			listenerHost: listenerHostname("*.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"http.example.com",
			},
			want:      map[string]struct{}{},
			wantError: []error{fmt.Errorf("gateway hostname \"*.projectcontour.io\" does not match route hostname \"http.example.com\"")},
		},
		"listener host & wildcard hostnames host do not match": {
			listenerHost: listenerHostname("http.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"*.projectcontour.io",
			},
			want:      map[string]struct{}{},
			wantError: []error{fmt.Errorf("gateway hostname \"http.projectcontour.io\" does not match route hostname \"*.projectcontour.io\"")},
		},
		"listener host & wildcard hostname and matching hostname match": {
			listenerHost: listenerHostname("http.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"*.projectcontour.io",
				"http.projectcontour.io",
			},
			want: map[string]struct{}{
				"http.projectcontour.io": {},
			},
			wantError: []error{fmt.Errorf("gateway hostname \"http.projectcontour.io\" does not match route hostname \"*.projectcontour.io\"")},
		},
		"listener host & wildcard hostname and non-matching hostname don't match": {
			listenerHost: listenerHostname("http.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"*.projectcontour.io",
				"not.matching.io",
			},
			want: map[string]struct{}{},
			wantError: []error{
				fmt.Errorf("gateway hostname \"http.projectcontour.io\" does not match route hostname \"*.projectcontour.io\""),
				fmt.Errorf("gateway hostname \"http.projectcontour.io\" does not match route hostname \"not.matching.io\""),
			},
		},
		"listener host wildcard & wildcard hostnames host match": {
			listenerHost: listenerHostname("*.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"*.projectcontour.io",
			},
			want: map[string]struct{}{
				"*.projectcontour.io": {},
			},
			wantError: nil,
		},
		"listener wildcard matchall host & host match": {
			listenerHost: listenerHostname("*"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"http.projectcontour.io",
			},
			want: map[string]struct{}{
				"http.projectcontour.io": {},
			},
			wantError: nil,
		},
		"listener wildcard matchall host & multiple host match": {
			listenerHost: listenerHostname("*"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"http.projectcontour.io",
				"http2.projectcontour.io",
				"http3.projectcontour.io",
			},
			want: map[string]struct{}{
				"http.projectcontour.io":  {},
				"http2.projectcontour.io": {},
				"http3.projectcontour.io": {},
			},
			wantError: nil,
		},
		"listener host & hostname not defined match": {
			listenerHost: listenerHostname("http.projectcontour.io"),
			hostnames:    []gatewayapi_v1alpha1.Hostname{},
			want: map[string]struct{}{
				"http.projectcontour.io": {},
			},
			wantError: nil,
		},
		"listener host with many labels doesn't match hostnames wildcard host": {
			listenerHost: listenerHostname("too.many.labels.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"*.projectcontour.io",
			},
			want:      map[string]struct{}{},
			wantError: []error{fmt.Errorf("gateway hostname \"too.many.labels.projectcontour.io\" does not match route hostname \"*.projectcontour.io\"")},
		},
		"listener wildcard host doesn't match hostnames with many labels host": {
			listenerHost: listenerHostname("*.projectcontour.io"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"too.many.labels.projectcontour.io",
			},
			want:      map[string]struct{}{},
			wantError: []error{fmt.Errorf("gateway hostname \"*.projectcontour.io\" does not match route hostname \"too.many.labels.projectcontour.io\"")},
		},
		"listener wildcard host with wildcard hostname": {
			listenerHost: listenerHostname("*"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"*.projectcontour.io",
			},
			want: map[string]struct{}{
				"*.projectcontour.io": {},
			},
			wantError: nil,
		},
		"listener wildcard host with invalid wildcard hostname": {
			listenerHost: listenerHostname("*"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"*",
			},
			want:      map[string]struct{}{},
			wantError: []error{fmt.Errorf("invalid hostname \"*\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]")},
		},
		"listener wildcard host with bare hostname": {
			listenerHost: listenerHostname("*"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"foo",
			},
			want: map[string]struct{}{
				"foo": {},
			},
			wantError: nil,
		},
		"listener wildcard host with bare hostname is invalid": {
			listenerHost: listenerHostname("*.foo"),
			hostnames: []gatewayapi_v1alpha1.Hostname{
				"foo",
			},
			want:      map[string]struct{}{},
			wantError: []error{fmt.Errorf("gateway hostname \"*.foo\" does not match route hostname \"foo\"")},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
			}

			got, gotError := processor.computeHosts(tc.hostnames, tc.listenerHost)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantError, gotError)
		})
	}
}

func listenerHostname(host string) *gatewayapi_v1alpha1.Hostname {
	h := gatewayapi_v1alpha1.Hostname(host)
	return &h
}

func TestNamespaceMatches(t *testing.T) {
	tests := map[string]struct {
		namespaces *gatewayapi_v1alpha1.RouteNamespaces
		namespace  string
		valid      bool
		wantError  bool
	}{
		"nil matches all": {
			namespaces: nil,
			namespace:  "projectcontour",
			valid:      true,
			wantError:  false,
		},
		"nil From matches all": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: nil,
			},
			namespace: "projectcontour",
			valid:     true,
			wantError: false,
		},
		"From.RouteSelectAll matches all": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
			},
			namespace: "projectcontour",
			valid:     true,
			wantError: false,
		},
		"From.RouteSelectSame matches": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSame),
			},
			namespace: "projectcontour",
			valid:     true,
			wantError: false,
		},
		"From.RouteSelectSame doesn't match": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSame),
			},
			namespace: "custom",
			valid:     false,
			wantError: false,
		},
		"From.RouteSelectSelector matches labels, same ns as gateway": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "production",
					},
				},
			},
			namespace: "projectcontour",
			valid:     true,
			wantError: false,
		},
		"From.RouteSelectSelector matches labels, different ns as gateway": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"something": "special",
					},
				},
			},
			namespace: "custom",
			valid:     true,
			wantError: false,
		},
		"From.RouteSelectSelector doesn't matches labels, different ns as gateway": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"something": "special",
					},
				},
			},
			namespace: "projectcontour",
			valid:     false,
			wantError: false,
		},
		"From.RouteSelectSelector matches expression 'In', different ns as gateway": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"special"},
					}},
				},
			},
			namespace: "custom",
			valid:     true,
			wantError: false,
		},
		"From.RouteSelectSelector matches expression 'DoesNotExist', different ns as gateway": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "notthere",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					}},
				},
			},
			namespace: "custom",
			valid:     true,
			wantError: false,
		},
		"From.RouteSelectSelector doesn't match expression 'DoesNotExist', different ns as gateway": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					}},
				},
			},
			namespace: "custom",
			valid:     false,
			wantError: false,
		},
		"From.RouteSelectSelector matches expression 'Exists', different ns as gateway": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "notthere",
						Operator: metav1.LabelSelectorOpExists,
					}},
				},
			},
			namespace: "custom",
			valid:     false,
			wantError: false,
		},
		"From.RouteSelectSelector doesn't match expression 'Exists', different ns as gateway": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: metav1.LabelSelectorOpExists,
					}},
				},
			},
			namespace: "custom",
			valid:     true,
			wantError: false,
		},
		"From.RouteSelectSelector match expression 'Exists', cannot specify values": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: metav1.LabelSelectorOpExists,
						Values:   []string{"error"},
					}},
				},
			},
			namespace: "custom",
			valid:     false,
			wantError: true,
		},
		"From.RouteSelectSelector match expression 'NotExists', cannot specify values": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "something",
						Operator: metav1.LabelSelectorOpDoesNotExist,
						Values:   []string{"error"},
					}},
				},
			},
			namespace: "custom",
			valid:     false,
			wantError: true,
		},
		"From.RouteSelectSelector must define matchLabels or matchExpression": {
			namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
				From:     routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectSelector),
				Selector: &metav1.LabelSelector{},
			},
			namespace: "custom",
			valid:     false,
			wantError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
				source: &KubernetesCache{
					gateway: &gatewayapi_v1alpha1.Gateway{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "contour",
							Namespace: "projectcontour",
						},
					},
					namespaces: map[string]*v1.Namespace{
						"projectcontour": {
							ObjectMeta: metav1.ObjectMeta{
								Name: "projectcontour",
								Labels: map[string]string{
									"app": "production",
								},
							},
						},
						"custom": {
							ObjectMeta: metav1.ObjectMeta{
								Name: "custom",
								Labels: map[string]string{
									"something": "special",
									"another":   "val",
									"testkey":   "testval",
								},
							},
						},
						"customsimilar": {
							ObjectMeta: metav1.ObjectMeta{
								Name: "custom",
								Labels: map[string]string{
									"something": "special",
								},
							},
						},
					},
				},
			}

			got, gotError := processor.namespaceMatches(tc.namespaces, tc.namespace)
			assert.Equal(t, tc.valid, got)
			assert.Equal(t, tc.wantError, gotError != nil)
		})
	}
}

func TestGatewayMatches(t *testing.T) {
	tests := map[string]struct {
		routeGateways *gatewayapi_v1alpha1.RouteGateways
		namespace     string
		want          bool
	}{
		"gateway allow all is always valid": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowAll),
			},
			namespace: "",
			want:      true,
		},
		"gateway allow from list matches configured gateway": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowFromList),
				GatewayRefs: []gatewayapi_v1alpha1.GatewayReference{{
					Name:      "contour",
					Namespace: "projectcontour",
				}},
			},
			namespace: "projectcontour",
			want:      true,
		},
		"gateway allow from list doesn't match configured gateway": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowFromList),
				GatewayRefs: []gatewayapi_v1alpha1.GatewayReference{{
					Name:      "different",
					Namespace: "gateway",
				}},
			},
			namespace: "projectcontour",
			want:      false,
		},
		"gateway allow same namespace matches configured gateway": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
			},
			namespace: "projectcontour",
			want:      true,
		},
		"gateway allow same namespace doesn't match configured gateway": {
			routeGateways: &gatewayapi_v1alpha1.RouteGateways{
				Allow: gatewayAllowTypePtr(gatewayapi_v1alpha1.GatewayAllowSameNamespace),
			},
			namespace: "different",
			want:      false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			processor := &GatewayAPIProcessor{
				FieldLogger: fixture.NewTestLogger(t),
				source: &KubernetesCache{
					gateway: &gatewayapi_v1alpha1.Gateway{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "contour",
							Namespace: "projectcontour",
						},
					},
				},
			}

			got := processor.gatewayMatches(tc.routeGateways, tc.namespace)
			assert.Equal(t, tc.want, got)
		})
	}
}
