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
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestVirtualHostValid(t *testing.T) {

	vh := VirtualHost{}
	assert.False(t, vh.Valid())

	vh = VirtualHost{
		routes: map[string]*Route{
			"/": {},
		},
	}
	assert.True(t, vh.Valid())
}

func TestSecureVirtualHostValid(t *testing.T) {

	vh := SecureVirtualHost{}
	assert.False(t, vh.Valid())

	sec := new(Secret)
	vh = SecureVirtualHost{
		Secrets: map[string]*Secret{sec.Name(): sec},
	}
	assert.False(t, vh.Valid())

	vh = SecureVirtualHost{
		VirtualHost: VirtualHost{
			routes: map[string]*Route{
				"/": {},
			},
		},
	}
	assert.False(t, vh.Valid())

	sec = new(Secret)
	vh = SecureVirtualHost{
		Secrets: map[string]*Secret{sec.Name(): sec},
		VirtualHost: VirtualHost{
			routes: map[string]*Route{
				"/": {},
			},
		},
	}
	assert.True(t, vh.Valid())

	vh = SecureVirtualHost{
		TCPProxy: new(TCPProxy),
	}
	assert.True(t, vh.Valid())

	sec = new(Secret)
	vh = SecureVirtualHost{
		Secrets:  map[string]*Secret{sec.Name(): sec},
		TCPProxy: new(TCPProxy),
	}
	assert.True(t, vh.Valid())
}

func TestPeerValidationContext(t *testing.T) {
	pvc1 := PeerValidationContext{
		CACertificate: &Secret{
			Object: &v1.Secret{
				Data: map[string][]byte{
					CACertificateKey: []byte("cacert"),
				},
			},
		},
		SubjectName: "subject",
	}
	pvc2 := PeerValidationContext{}
	var pvc3 *PeerValidationContext

	assert.Equal(t, pvc1.GetSubjectName(), "subject")
	assert.Equal(t, pvc1.GetCACertificate(), []byte("cacert"))
	assert.Equal(t, pvc2.GetSubjectName(), "")
	assert.Equal(t, pvc2.GetCACertificate(), []byte(nil))
	assert.Equal(t, pvc3.GetSubjectName(), "")
	assert.Equal(t, pvc3.GetCACertificate(), []byte(nil))
}

func TestObserverFunc(t *testing.T) {
	// Ensure nil doesn't panic.
	ObserverFunc(nil).OnChange(nil)

	// Ensure the given function gets called.
	result := false
	ObserverFunc(func(*DAG) { result = true }).OnChange(nil)
	assert.Equal(t, true, result)
}

func TestServiceClusterValid(t *testing.T) {
	invalid := []ServiceCluster{
		{},
		{ClusterName: "foo"},
		{ClusterName: "foo", Services: []WeightedService{{}}},
		{ClusterName: "foo", Services: []WeightedService{{ServiceName: "foo"}}},
		{ClusterName: "foo", Services: []WeightedService{{ServiceNamespace: "foo"}}},
	}

	for _, c := range invalid {
		assert.Errorf(t, c.Validate(), "invalid cluster %#v", c)
	}
}

func TestServiceClusterAdd(t *testing.T) {
	port := v1.ServicePort{
		Name:     "foo",
		Protocol: v1.ProtocolTCP,
		Port:     32,
	}

	s := ServiceCluster{
		ClusterName: "test",
	}

	s.AddService(types.NamespacedName{Namespace: "ns", Name: "s1"}, port)
	assert.Equal(t,
		ServiceCluster{
			ClusterName: "test",
			Services: []WeightedService{{
				Weight:           1,
				ServiceName:      "s1",
				ServiceNamespace: "ns",
				ServicePort:      port,
			}},
		},
		s)

	s.AddWeightedService(9, types.NamespacedName{Namespace: "ns", Name: "s2"}, port)
	assert.Equal(t,
		ServiceCluster{
			ClusterName: "test",
			Services: []WeightedService{{
				Weight:           1,
				ServiceName:      "s1",
				ServiceNamespace: "ns",
				ServicePort:      port,
			}, {
				Weight:           9,
				ServiceName:      "s2",
				ServiceNamespace: "ns",
				ServicePort:      port,
			}},
		},
		s)
}

func TestServiceClusterRebalance(t *testing.T) {
	port := v1.ServicePort{
		Name:     "foo",
		Protocol: v1.ProtocolTCP,
		Port:     32,
	}

	cases := map[string]struct {
		have ServiceCluster
		want ServiceCluster
	}{
		"default weights": {
			have: ServiceCluster{
				ClusterName: "test",
				Services: []WeightedService{{
					ServiceName:      "s1",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}, {
					ServiceName:      "s2",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}},
			},
			want: ServiceCluster{
				ClusterName: "test",
				Services: []WeightedService{{
					Weight:           1,
					ServiceName:      "s1",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}, {
					Weight:           1,
					ServiceName:      "s2",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}},
			},
		},
		"custom weights": {
			have: ServiceCluster{
				ClusterName: "test",
				Services: []WeightedService{{
					ServiceName:      "s1",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}, {
					Weight:           6,
					ServiceName:      "s2",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}},
			},
			want: ServiceCluster{
				ClusterName: "test",
				Services: []WeightedService{{
					Weight:           0,
					ServiceName:      "s1",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}, {
					Weight:           6,
					ServiceName:      "s2",
					ServiceNamespace: "ns",
					ServicePort:      port,
				}},
			},
		},
	}

	for n, c := range cases {
		t.Run(n, func(t *testing.T) {
			s := c.have
			s.Rebalance()
			assert.Equal(t, c.want, s)
		})
	}

}
