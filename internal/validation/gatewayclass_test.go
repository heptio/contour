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

package validation

import (
	"context"
	"testing"

	"github.com/projectcontour/contour/internal/k8s"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var (
	ctx = context.TODO()

	defaultController = "projectcontour.io/projectcontour/contour"

	new = &gatewayapi_v1alpha1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "new",
		},
		Spec: gatewayapi_v1alpha1.GatewayClassSpec{
			Controller: defaultController,
		},
	}

	old = &gatewayapi_v1alpha1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "old",
		},
	}
)

func TestGatewayClass(t *testing.T) {
	testCases := map[string]struct {
		mutateNew func(gc *gatewayapi_v1alpha1.GatewayClass)
		mutateOld func(gc *gatewayapi_v1alpha1.GatewayClass)
		errType   field.ErrorType
		errField  string
		expect    bool
	}{
		"valid gatewayclass": {
			mutateNew: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			expect:    true,
		},
		"invalid name": {
			mutateNew: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Name = "invalid name"
			},
			errType:  field.ErrorTypeInvalid,
			errField: "metadata.name",
			expect:   false,
		},
		"existing admitted gatewayclass with same controller": {
			mutateNew: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			mutateOld: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Spec.Controller = defaultController
				gc.Status = gatewayapi_v1alpha1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
							Status: metav1.ConditionTrue,
						},
					},
				}
			},
			errType:  field.ErrorTypeInternal,
			errField: "spec.controller",
			expect:   false,
		},
		"existing non-admitted gatewayclass with same controller": {
			mutateNew: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			mutateOld: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Spec.Controller = defaultController
				gc.Status = gatewayapi_v1alpha1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
							Status: metav1.ConditionFalse,
						},
					},
				}
			},
			expect: true,
		},
		"existing gatewayclass with different controller": {
			mutateNew: func(_ *gatewayapi_v1alpha1.GatewayClass) {},
			mutateOld: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Spec.Controller = "foo.io/bar"
				gc.Status = gatewayapi_v1alpha1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1alpha1.GatewayClassConditionStatusAdmitted),
							Status: metav1.ConditionTrue,
						},
					},
				}
			},
			expect: true,
		},
		"gatewayclass paramsRef specified": {
			mutateNew: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Name = "gatewayclass-params-specified"
				gc.Spec.ParametersRef = &gatewayapi_v1alpha1.ParametersReference{
					Group: "foo",
					Kind:  "bar",
					Name:  "baz",
				}
			},
			expect: true,
		},
		"gatewayclass paramsRef not specified": {
			mutateNew: func(gc *gatewayapi_v1alpha1.GatewayClass) {
				gc.Name = "gatewayclass-params-not-specified"
			},
			expect: true,
		},
	}

	// Build the client
	builder := fake.NewClientBuilder()
	scheme, err := k8s.NewContourScheme()
	if err != nil {
		t.Fatalf("failed to build contour scheme: %v", err)
	}
	builder.WithScheme(scheme)

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Create the client
			cl := builder.Build()

			newMutated := new.DeepCopy()
			tc.mutateNew(newMutated)

			err := cl.Create(context.TODO(), newMutated)
			require.NoErrorf(t, err, "Failed to create gatewayclass %s", newMutated.Name)

			if tc.mutateOld != nil {
				oldMutated := old.DeepCopy()
				tc.mutateOld(oldMutated)

				err := cl.Create(context.TODO(), oldMutated)
				require.NoErrorf(t, err, "Failed to create gatewayclass %s", oldMutated.Name)
			}

			actual := ValidateGatewayClass(ctx, cl, newMutated, newMutated.Spec.Controller)
			if tc.expect {
				assert.Nilf(t, actual, "expected no error, got: %v", actual)
			} else {
				// Not asserting an error since a field.ErrorList is being returned.
				assert.NotNilf(t, actual, "expected no error, got: %v", actual)
				for _, a := range actual {
					assert.Contains(t, a.Type, tc.errType)
					assert.Contains(t, a.Field, tc.errField)
				}
			}
		})
	}
}
