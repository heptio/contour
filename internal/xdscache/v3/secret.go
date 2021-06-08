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

package v3

import (
	"sort"
	"sync"

	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// SecretCache manages the contents of the gRPC SDS cache.
type SecretCache struct {
	mu     sync.Mutex
	values map[string]*envoy_tls_v3.Secret
	contour.Cond
}

// Update replaces the contents of the cache with the supplied map.
func (c *SecretCache) Update(v map[string]*envoy_tls_v3.Secret) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.Cond.Notify()
}

// Contents returns a copy of the cache's contents.
func (c *SecretCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_tls_v3.Secret
	for _, v := range c.values {
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (c *SecretCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_tls_v3.Secret
	for _, n := range names {
		// we can only return secrets where their value is
		// known. if the secret is not registered in the cache
		// we return nothing.
		if v, ok := c.values[n]; ok {
			values = append(values, v)
		}
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*SecretCache) TypeURL() string { return resource.SecretType }

func (c *SecretCache) OnChange(root *dag.DAG) {
	secrets := visitSecrets(root)
	c.Update(secrets)
}

type secretVisitor struct {
	secrets map[string]*envoy_tls_v3.Secret
}

// visitSecrets produces a map of *envoy_tls_v3.Secret
func visitSecrets(root dag.Vertex) map[string]*envoy_tls_v3.Secret {
	sv := secretVisitor{
		secrets: make(map[string]*envoy_tls_v3.Secret),
	}
	sv.visit(root)
	return sv.secrets
}

func (v *secretVisitor) addSecret(s *dag.Secret) {
	name := envoy.Secretname(s)
	if _, ok := v.secrets[name]; !ok {
		envoySecret := envoy_v3.Secret(s)
		v.secrets[envoySecret.Name] = envoySecret
	}
}

func (v *secretVisitor) visit(vertex dag.Vertex) {
	switch obj := vertex.(type) {
	case *dag.SecureVirtualHost:
		if len(obj.Secrets) > 0 {
			for _, secret := range obj.Secrets {
				v.addSecret(secret)
			}
		}
		if obj.FallbackCertificate != nil {
			v.addSecret(obj.FallbackCertificate)
		}
	case *dag.Cluster:
		if obj.ClientCertificate != nil {
			v.addSecret(obj.ClientCertificate)
		}
	default:
		vertex.Visit(v.visit)
	}
}
