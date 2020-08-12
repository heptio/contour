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

// Package assert provides assertion helpers
package assert

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	tassert "github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	Equal         = tassert.Equal
	Equalf        = tassert.Equalf
	ElementsMatch = tassert.ElementsMatch
)

// EqualProto will test that want == got, and call t.Fatal if it does not.
// Notably, for errors, they are equal if they are both nil, or are both non-nil.
// No value information is checked for errors.
//
// This is a different behavior to the aliased Testify functions - historically,
// the featuretests expected this behavior, and this is mainly used there.
// So we've kept it for now.
// TODO(youngnick): talk to me if you want to fix this.
func EqualProto(t *testing.T, want, got interface{}, msgAndArgs ...interface{}) bool {
	t.Helper()

	diff := cmp.Diff(want, got, protocmp.Transform())
	if diff != "" {
		t.Error(diff)
		return false
	}

	return true
}
