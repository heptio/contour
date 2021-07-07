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

package build

import (
	"gopkg.in/yaml.v2"
)

// BuildInfo is a struct for build information.
// nolint:revive
type BuildInfo struct {
	Branch  string `yaml:"branch,omitempty"`
	Sha     string `yaml:"sha,omitempty"`
	Version string `yaml:"version,omitempty"`
}

// Branch allows for a queryable branch name set at build time.
var Branch string

// Sha allows for a queryable git sha set at build time.
var Sha string

// Version allows for a queryable version set at build time.
var Version string

// PrintBuildInfo prints the build information.
func PrintBuildInfo() string {
	buildInfo := &BuildInfo{Branch, Sha, Version}
	out, err := yaml.Marshal(buildInfo)
	if err != nil {
		panic(err)
	}
	return string(out)
}
