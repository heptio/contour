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
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/status"
	"k8s.io/apimachinery/pkg/types"
)

// Processor constructs part of a DAG.
type Processor interface {
	// Run executes the processor.
	Run(dag *DAG, source *KubernetesCache)
}

// ProcessorFunc adapts a function to the Processor interface.
type ProcessorFunc func(*DAG, *KubernetesCache)

func (pf ProcessorFunc) Run(dag *DAG, source *KubernetesCache) {
	if pf != nil {
		pf(dag, source)
	}
}

// Builder builds a DAG.
type Builder struct {
	// Source is the source of Kubernetes objects
	// from which to build a DAG.
	Source KubernetesCache

	// Processors is the ordered list of Processors to
	// use to build the DAG.
	Processors []Processor
}

// Build builds and returns a new DAG by running the
// configured DAG processors, in order.
func (b *Builder) Build() *DAG {

	gatewayNSName := types.NamespacedName{}
	if b.Source.gateway != nil {
		gatewayNSName = k8s.NamespacedNameOf(b.Source.gateway)
	}

	dag := DAG{
		StatusCache: status.NewCache(gatewayNSName),
	}

	for _, p := range b.Processors {
		p.Run(&dag, &b.Source)
	}
	return &dag
}
