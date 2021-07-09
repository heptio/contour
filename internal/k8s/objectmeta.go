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

package k8s

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NamespacedNameOf returns the NamespacedName of any given Kubernetes object.
func NamespacedNameOf(obj metav1.Object) types.NamespacedName {

	name := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	if name.Namespace == "" {
		name.Namespace = metav1.NamespaceDefault
	}

	return name
}

// DefaultNamespace can be used with NamespacedNameFrom to set the
// default namespace for a resource name that may not be qualified by
// a namespace.
func DefaultNamespace(ns string) func(name *types.NamespacedName) {
	return func(name *types.NamespacedName) {
		if name.Namespace == "" {
			name.Namespace = ns
		}
	}
}

// NamespacedNameFrom parses a resource name string into a fully qualified NamespacedName.
func NamespacedNameFrom(nameStr string, opts ...func(*types.NamespacedName)) types.NamespacedName {
	var name types.NamespacedName

	v := strings.SplitN(nameStr, "/", 2)
	switch len(v) {
	case 1:
		// No '/' separator.
		name = types.NamespacedName{
			Name:      v[0],
			Namespace: "",
		}
	default:
		name = types.NamespacedName{
			Name:      v[1],
			Namespace: v[0],
		}
	}

	for _, o := range opts {
		o(&name)
	}

	if name.Namespace == "" {
		name.Namespace = metav1.NamespaceDefault
	}

	return name
}
