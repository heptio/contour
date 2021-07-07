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

package incluster

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
)

var f = e2e.NewFramework(true)

func TestIncluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Incluster tests")
}

var _ = BeforeSuite(func() {
	require.NoError(f.T(), f.Deployment.EnsureResourcesForInclusterContour())
})

var _ = AfterSuite(func() {
	f.DeleteNamespace(f.Deployment.Namespace.Name, true)
})

var _ = Describe("Incluster", func() {
	f.NamespacedTest("projectcontour-resource-rbac", testProjectcontourResourcesRBAC)

	f.NamespacedTest("ingress-resource-rbac", testIngressResourceRBAC)
})
