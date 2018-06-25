// Copyright © 2018 Heptio
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

// Package dag provides a data model, in the form of a directed acyclic graph,
// of the relationship between Kubernetes Ingress, Service, and Secret objects.
package dag

import (
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"

	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
)

// A DAG represents a directed acylic graph of objects representing the relationship
// between Kubernetes Ingress objects, the backend Services, and Secret objects.
// The DAG models these relationships as Roots and Vertices.
type DAG struct {
	mu sync.Mutex

	ingresses     map[meta]*v1beta1.Ingress
	ingressroutes map[meta]*ingressroutev1.IngressRoute
	secrets       map[meta]*v1.Secret
	services      map[meta]*v1.Service

	dag *dag
}

// dag represents
type dag struct {
	// roots are the roots of this dag
	roots []Vertex
}

// meta holds the name and namespace of a Kubernetes object.
type meta struct {
	name, namespace string
}

// Visit calls f for every root of this DAG.
func (d *DAG) Visit(f func(Vertex)) {
	d.mu.Lock()
	dag := d.dag
	d.mu.Unlock()
	for _, r := range dag.roots {
		f(r)
	}
}

// Insert inserts obj into the DAG. If an object with a matching type, name, and
// namespace exists, it will be overwritten.
func (d *DAG) Insert(obj interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if d.secrets == nil {
			d.secrets = make(map[meta]*v1.Secret)
		}
		d.secrets[m] = obj
	case *v1.Service:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if d.services == nil {
			d.services = make(map[meta]*v1.Service)
		}
		d.services[m] = obj
	case *v1beta1.Ingress:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if d.ingresses == nil {
			d.ingresses = make(map[meta]*v1beta1.Ingress)
		}
		d.ingresses[m] = obj

	case *ingressroutev1.IngressRoute:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		if d.ingressroutes == nil {
			d.ingressroutes = make(map[meta]*ingressroutev1.IngressRoute)
		}
		d.ingressroutes[m] = obj
	default:
		// not an interesting object
	}
}

// Remove removes obj from the DAG. If no object with a matching type, name, and
// namespace exists in the DAG, no action is taken.
func (d *DAG) Remove(obj interface{}) {
	switch obj := obj.(type) {
	default:
		d.remove(obj)
	case cache.DeletedFinalStateUnknown:
		d.Remove(obj.Obj) // recurse into ourselves with the tombstoned value
	}
}

func (d *DAG) remove(obj interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch obj := obj.(type) {
	case *v1.Secret:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(d.secrets, m)
	case *v1.Service:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(d.services, m)
	case *v1beta1.Ingress:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(d.ingresses, m)
	case *ingressroutev1.IngressRoute:
		m := meta{name: obj.Name, namespace: obj.Namespace}
		delete(d.ingressroutes, m)
	default:
		// not interesting
	}
}

// Recompute recomputes the DAG.
func (d *DAG) Recompute() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dag = d.recompute()
}

// recompute builds a new *dag.dag.
func (d *DAG) recompute() *dag {
	// memoise access to a service map, built
	// as needed from the list of services cached
	// from k8s.
	_services := make(map[meta]*Service)
	service := func(m meta) *Service {
		if s, ok := _services[m]; ok {
			return s
		}
		svc, ok := d.services[m]
		if !ok {
			return nil
		}
		s := &Service{
			object: svc,
		}
		_services[s.toMeta()] = s
		return s
	}

	// memoise access to a secrets map, built
	// as needed from the list of secrets cached
	// from k8s.
	_secrets := make(map[meta]*Secret)
	secret := func(m meta) *Secret {
		if s, ok := _secrets[m]; ok {
			return s
		}
		sec, ok := d.secrets[m]
		if !ok {
			return nil
		}
		s := &Secret{
			object: sec,
		}
		_secrets[s.toMeta()] = s
		return s
	}

	type hostport struct {
		host string
		port int
	}

	// memoise the production of vhost entries as needed.
	_vhosts := make(map[hostport]*VirtualHost)
	vhost := func(host string, port int) *VirtualHost {
		hp := hostport{host: host, port: port}
		vh, ok := _vhosts[hp]
		if !ok {
			vh = &VirtualHost{
				Port:   port,
				host:   host,
				routes: make(map[string]*Route),
			}
			_vhosts[hp] = vh
		}
		return vh
	}

	// deconstruct each ingress into routes and virtualhost entries
	for _, ing := range d.ingresses {
		// should we create port 80 routes for this ingress
		httpAllowed := httpAllowed(ing)

		if ing.Spec.Backend != nil {
			// handle the annoying default ingress
			r := &Route{
				path:    "/",
				object:  ing,
				backend: ing.Spec.Backend,
			}
			m := meta{name: ing.Spec.Backend.ServiceName, namespace: ing.Namespace}
			if s := service(m); s != nil {
				// iterate through the ports on the service object, if we
				// find a match against the backends port's name or number, we add
				// the service as a child of the route.
				for _, p := range s.object.Spec.Ports {
					if ing.Spec.Backend.ServicePort.IntValue() == int(p.Port) || ing.Spec.Backend.ServicePort.String() == p.Name {
						r.addService(s)
					}
				}
			}
			if httpAllowed {
				vhost("*", 80).routes[r.path] = r
			}
		}

		// attach secrets from ingress to vhosts
		for _, tls := range ing.Spec.TLS {
			m := meta{name: tls.SecretName, namespace: ing.Namespace}
			if sec := secret(m); sec != nil {
				for _, host := range tls.Hosts {
					vhost(host, 443).addSecret(sec)
				}
			}
		}

		for _, rule := range ing.Spec.Rules {
			// handle Spec.Rule declarations
			host := rule.Host
			if host == "" {
				host = "*"
			}
			for n := range rule.IngressRuleValue.HTTP.Paths {
				path := rule.IngressRuleValue.HTTP.Paths[n].Path
				if path == "" {
					path = "/"
				}
				r := &Route{
					path:    path,
					object:  ing,
					backend: &rule.IngressRuleValue.HTTP.Paths[n].Backend,
				}

				m := meta{name: rule.IngressRuleValue.HTTP.Paths[n].Backend.ServiceName, namespace: ing.Namespace}
				if s := service(m); s != nil {
					// iterate through the ports on the service object, if we
					// find a match against the backends port's name or number, we add
					// the service as a child of the route.
					for _, p := range s.object.Spec.Ports {
						if rule.IngressRuleValue.HTTP.Paths[n].Backend.ServicePort.IntValue() == int(p.Port) || rule.IngressRuleValue.HTTP.Paths[n].Backend.ServicePort.String() == p.Name {
							r.addService(s)
						}
					}
				}
				if httpAllowed {
					vhost(host, 80).routes[r.path] = r
				}
				if _, ok := _vhosts[hostport{host: host, port: 443}]; ok && host != "*" {
					vhost(host, 443).routes[r.path] = r
				}
			}
		}
	}

	// process ingressroute documents
	for _, ir := range d.ingressroutes {
		if ir.Spec.VirtualHost == nil {
			// delegate ingressroute, skip it
			continue
		}

		host := ir.Spec.VirtualHost.Fqdn

		if tls := ir.Spec.VirtualHost.TLS; tls != nil {
			// attach secrets to TLS enabled vhosts
			m := meta{name: tls.SecretName, namespace: ir.Namespace}
			if sec := secret(m); sec != nil {
				vhost(host, 443).addSecret(sec)
			}
		}

		// attach routes to vhost
		for _, route := range ir.Spec.Routes {
			path := route.Match
			if path == "" {
				path = "/"
			}
			r := &Route{
				path:   path,
				object: ir,
			}
			for _, s := range route.Services {
				m := meta{name: s.Name, namespace: ir.Namespace}
				if svc := service(m); svc != nil {
					// iterate through the ports on the service object, if we
					// find a match against the backends port's name or number, we add
					// the service as a child of the route.
					for _, p := range svc.object.Spec.Ports {
						if s.Port == int(p.Port) {
							r.addService(svc)
						}
					}
				}
			}
			vhost(host, 80).routes[path] = r
		}
	}

	// append each computed vhost as a root of the dag.
	// this may include vhosts without routes, only secrets,
	// this is something a walker will have to be aware of.
	_d := new(dag)
	for _, vh := range _vhosts {
		_d.roots = append(_d.roots, vh)
	}
	return _d
}

type Root interface {
	Vertex
}

type Route struct {
	path     string
	object   interface{} // one of Ingress or IngressRoute
	services map[meta]*Service
	backend  interface{} // one of *v1beta1.IngressBackend or ingressroute.service
}

func (r *Route) Prefix() string { return r.path }
func (r *Route) ServicePort() string {
	switch b := r.backend.(type) {
	case *v1beta1.IngressBackend:
		return b.ServicePort.String()
	default:
		return "unknown"
	}
}

func (r *Route) addService(s *Service) {
	if r.services == nil {
		r.services = make(map[meta]*Service)
	}
	r.services[s.toMeta()] = s
}

func (r *Route) Visit(f func(Vertex)) {
	for _, c := range r.services {
		f(c)
	}
}

// A VirtualHost describes a Vertex that represents the root
// of a tree of objects associated with a HTTP Host: header.
type VirtualHost struct {
	// Port is the port that the VirtualHost will listen on.
	// Expected values are 80 and 443, but others are possible
	// if the VirtualHost is generated inside Contour.
	Port int

	host    string
	routes  map[string]*Route
	secrets map[meta]*Secret
}

func (v *VirtualHost) FQDN() string { return v.host }

func (v *VirtualHost) Visit(f func(Vertex)) {
	for _, r := range v.routes {
		f(r)
	}
	for _, s := range v.secrets {
		f(s)
	}
}

func (v *VirtualHost) addSecret(s *Secret) {
	if v.secrets == nil {
		v.secrets = make(map[meta]*Secret)
	}
	v.secrets[s.toMeta()] = s
}

type Vertex interface {
	Visit(func(Vertex))
}

// Secret represents a K8s Sevice as a DAG vertex. A Serivce is
// a leaf in the DAG.
type Service struct {
	object *v1.Service
}

func (s *Service) Name() string       { return s.object.Name }
func (s *Service) Namespace() string  { return s.object.Namespace }
func (s *Service) Visit(func(Vertex)) {}

func (s *Service) toMeta() meta {
	return meta{
		name:      s.object.Name,
		namespace: s.object.Namespace,
	}
}

// Secret represents a K8s Secret as a DAG Vertex. A Secret is
// a leaf in the DAG.
type Secret struct {
	object *v1.Secret
}

func (s *Secret) Name() string       { return s.object.Name }
func (s *Secret) Namespace() string  { return s.object.Namespace }
func (s *Secret) Visit(func(Vertex)) {}

func (s *Secret) toMeta() meta {
	return meta{
		name:      s.object.Name,
		namespace: s.object.Namespace,
	}
}
