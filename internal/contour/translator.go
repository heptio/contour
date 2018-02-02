// Copyright © 2017 Heptio
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

// Package contour contains the translation business logic that listens
// to Kubernetes ResourceEventHandler events and translates those into
// additions/deletions in caches connected to the Envoy xDS gRPC API server.
package contour

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	v2 "github.com/envoyproxy/go-control-plane/api"

	"github.com/heptio/contour/internal/log"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

type metadata struct {
	name, namespace string
}

// Translator receives notifications from the Kubernetes API and translates those
// objects into additions and removals entries of Envoy gRPC objects from a cache.
type Translator struct {
	// The logger for this Translator. There is no valid default, this value
	// must be supplied by the caller.
	log.Logger

	ClusterCache
	ClusterLoadAssignmentCache
	ListenerCache
	VirtualHostCache

	cache translatorCache
}

func (t *Translator) OnAdd(obj interface{}) {
	t.cache.OnAdd(obj)
	switch obj := obj.(type) {
	case *v1.Service:
		t.addService(obj)
	case *v1.Endpoints:
		t.addEndpoints(obj)
	case *v1beta1.Ingress:
		t.addIngress(obj)
	case *v1.Secret:
		t.addSecret(obj)
	default:
		t.Errorf("OnAdd unexpected type %T: %#v", obj, obj)
	}
}

func (t *Translator) OnUpdate(oldObj, newObj interface{}) {
	t.cache.OnUpdate(oldObj, newObj)
	// TODO(dfc) need to inspect oldObj and remove unused parts of the config from the cache.
	switch newObj := newObj.(type) {
	case *v1.Service:
		if oldObj, ok := oldObj.(*v1.Service); ok {
			t.recomputeService(oldObj, newObj)
		} else {
			t.Errorf("OnUpdate service %#v received invalid oldObj %T: %#v", newObj, oldObj, oldObj)
		}
	case *v1.Endpoints:
		t.addEndpoints(newObj)
	case *v1beta1.Ingress:
		t.addIngress(newObj)
	case *v1.Secret:
		t.addSecret(newObj)
	default:
		t.Errorf("OnUpdate unexpected type %T: %#v", newObj, newObj)
	}
}

func (t *Translator) OnDelete(obj interface{}) {
	t.cache.OnDelete(obj)
	switch obj := obj.(type) {
	case *v1.Service:
		t.removeService(obj)
	case *v1.Endpoints:
		t.removeEndpoints(obj)
	case *v1beta1.Ingress:
		t.removeIngress(obj)
	case *v1.Secret:
		t.removeSecret(obj)
	case cache.DeletedFinalStateUnknown:
		t.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		t.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
	}
}

func (t *Translator) addService(svc *v1.Service) {
	ep, ok := t.cache.endpoints[metadata{name: svc.Name, namespace: svc.Namespace}]
	if !ok {
		t.Infof("ignoring service update, cannot find matching endpoint %s/%s", svc.Namespace, svc.Name)
	} else {
		t.recomputeClusterLoadAssignment(nil, ep)
	}
	t.recomputeService(nil, svc)
}

func (t *Translator) removeService(svc *v1.Service) {
	t.recomputeService(svc, nil)
}

func (t *Translator) addEndpoints(e *v1.Endpoints) {
	if len(e.Subsets) < 1 {
		// if there are no endpoints in this object, ignore it
		// to avoid sending a noop notification to watchers.
		return
	}
	_, ok := t.cache.services[metadata{name: e.Name, namespace: e.Namespace}]
	if !ok {
		t.Infof("ignoring endpoint update, cannot find matchin service %s/%s", e.Namespace, e.Name)
		return
	}
	t.recomputeClusterLoadAssignment(nil, e)
}

func (t *Translator) removeEndpoints(e *v1.Endpoints) {
	t.recomputeClusterLoadAssignment(e, nil)
}

func (t *Translator) addIngress(i *v1beta1.Ingress) {
	class, ok := i.Annotations["kubernetes.io/ingress.class"]
	if ok && class != "contour" {
		// if there is an ingress class set, but it is not set to "contour"
		// ignore this ingress.
		// TODO(dfc) we should also skip creating any cluster backends,
		// but this is hard to do at the moment because cds and rds are
		// independent.
		return
	}

	t.recomputeListeners(t.cache.ingresses, t.cache.secrets)

	// notify watchers that the vhost cache has probably changed.
	defer t.VirtualHostCache.Notify()

	// handle the special case of the default ingress first.
	if i.Spec.Backend != nil {
		// update t.vhosts cache
		t.recomputevhost("*", t.cache.vhosts["*"])
	}

	for _, rule := range i.Spec.Rules {
		host := rule.Host
		if host == "" {
			// If the host is unspecified, the Ingress routes all traffic based on the specified IngressRuleValue.
			host = "*"
		}
		t.recomputevhost(host, t.cache.vhosts[host])
	}
}

func (t *Translator) removeIngress(i *v1beta1.Ingress) {
	class, ok := i.Annotations["kubernetes.io/ingress.class"]
	if ok && class != "contour" {
		// if there is an ingress class set, but it is not set to "contour"
		// ignore this ingress.
		// TODO(dfc) we should also skip creating any cluster backends,
		// but this is hard to do at the moment because cds and rds are
		// independent.
		return
	}

	defer t.VirtualHostCache.Notify()

	t.recomputeListeners(t.cache.ingresses, t.cache.secrets)

	if i.Spec.Backend != nil {
		t.recomputevhost("*", nil)
	}

	for _, rule := range i.Spec.Rules {
		host := rule.Host
		if host == "" {
			// If the host is unspecified, the Ingress routes all traffic based on the specified IngressRuleValue.
			host = "*"
		}
		t.recomputevhost(rule.Host, t.cache.vhosts[host])
	}
}

func (t *Translator) addSecret(s *v1.Secret) {
	_, cert := s.Data[v1.TLSCertKey]
	_, key := s.Data[v1.TLSPrivateKeyKey]
	if !cert || !key {
		t.Logger.Infof("ignoring secret %s/%s", s.Namespace, s.Name)
		return
	}
	t.Logger.Infof("caching secret %s/%s", s.Namespace, s.Name)
	t.writeCerts(s)

	t.recomputeTLSListener(t.cache.ingresses, t.cache.secrets)
}

func (t *Translator) removeSecret(s *v1.Secret) {
	t.recomputeTLSListener(t.cache.ingresses, t.cache.secrets)
}

// writeSecret writes the contents of the secret to a fixed location on
// disk so that envoy can pick them up.
// TODO(dfc) this is due to https://github.com/envoyproxy/envoy/issues/1357
func (t *Translator) writeCerts(s *v1.Secret) {
	const base = "/config/ssl"
	path := filepath.Join(base, s.Namespace, s.Name)
	if err := os.MkdirAll(path, 0644); err != nil {
		t.Errorf("could not write cert %s/%s: %v", s.Namespace, s.Name, err)
		return
	}
	if err := ioutil.WriteFile(filepath.Join(path, v1.TLSCertKey), s.Data[v1.TLSCertKey], 0755); err != nil {
		t.Errorf("could not write cert %s/%s: %v", s.Namespace, s.Name, err)
		return
	}
	if err := ioutil.WriteFile(filepath.Join(path, v1.TLSPrivateKeyKey), s.Data[v1.TLSPrivateKeyKey], 0755); err != nil {
		t.Errorf("could not write cert %s/%s: %v", s.Namespace, s.Name, err)
		return
	}
}

type translatorCache struct {
	ingresses map[metadata]*v1beta1.Ingress
	endpoints map[metadata]*v1.Endpoints
	services  map[metadata]*v1.Service

	// secrets stores tls secrets
	secrets map[metadata]*v1.Secret

	// vhosts stores a slice of vhosts with the ingress objects that
	// went into creating them.
	vhosts map[string]map[metadata]*v1beta1.Ingress
}

func (t *translatorCache) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		if t.services == nil {
			t.services = make(map[metadata]*v1.Service)
		}
		t.services[metadata{name: obj.Name, namespace: obj.Namespace}] = obj
	case *v1.Endpoints:
		if t.endpoints == nil {
			t.endpoints = make(map[metadata]*v1.Endpoints)
		}
		t.endpoints[metadata{name: obj.Name, namespace: obj.Namespace}] = obj
	case *v1beta1.Ingress:
		if t.ingresses == nil {
			t.ingresses = make(map[metadata]*v1beta1.Ingress)
		}
		md := metadata{name: obj.Name, namespace: obj.Namespace}
		t.ingresses[md] = obj
		if t.vhosts == nil {
			t.vhosts = make(map[string]map[metadata]*v1beta1.Ingress)
		}
		if obj.Spec.Backend != nil {
			if _, ok := t.vhosts["*"]; !ok {
				t.vhosts["*"] = make(map[metadata]*v1beta1.Ingress)
			}
			t.vhosts["*"][md] = obj
		}
		for _, rule := range obj.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			if _, ok := t.vhosts[host]; !ok {
				t.vhosts[host] = make(map[metadata]*v1beta1.Ingress)
			}
			t.vhosts[host][md] = obj
		}
	case *v1.Secret:
		if t.secrets == nil {
			t.secrets = make(map[metadata]*v1.Secret)
		}
		t.secrets[metadata{name: obj.Name, namespace: obj.Namespace}] = obj
	default:
		// ignore
	}
}

func (t *translatorCache) OnUpdate(oldObj, newObj interface{}) {
	switch oldObj := oldObj.(type) {
	case *v1beta1.Ingress:
		// ingress objects are special because their contents can change
		// which affects the t.vhost cache. The simplest way is to model
		// update as delete, then add.
		t.OnDelete(oldObj)
	}
	t.OnAdd(newObj)
}

func (t *translatorCache) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		delete(t.services, metadata{name: obj.Name, namespace: obj.Namespace})
	case *v1.Endpoints:
		delete(t.endpoints, metadata{name: obj.Name, namespace: obj.Namespace})
	case *v1beta1.Ingress:
		md := metadata{name: obj.Name, namespace: obj.Namespace}
		delete(t.ingresses, md)
		delete(t.vhosts["*"], md)
		for _, rule := range obj.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			delete(t.vhosts[host], md)
			if len(t.vhosts[host]) == 0 {
				delete(t.vhosts, host)
			}
		}
		if len(t.vhosts["*"]) == 0 {
			delete(t.vhosts, "*")
		}
	case *v1.Secret:
		delete(t.secrets, metadata{name: obj.Name, namespace: obj.Namespace})
	case cache.DeletedFinalStateUnknown:
		t.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		// ignore
	}
}

// hashname takes a lenth l and a varargs of strings s and returns a string whose length
// which does not exceed l. Internally s is joined with strings.Join(s, "/"). If the
// combined length exceeds l then hashname truncates each element in s, starting from the
// end using a hash derived from the contents of s (not the current element). This process
// continues until the length of s does not exceed l, or all elements have been truncated.
// In which case, the entire string is replaced with a hash not exceeding the length of l.
func hashname(l int, s ...string) string {
	const shorthash = 6 // the length of the shorthash

	r := strings.Join(s, "/")
	if l > len(r) {
		// we're under the limit, nothing to do
		return r
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r)))
	for n := len(s) - 1; n >= 0; n-- {
		s[n] = truncate(l/len(s), s[n], hash[:shorthash])
		r = strings.Join(s, "/")
		if l > len(r) {
			return r
		}
	}
	// truncated everything, but we're still too long
	// just return the hash truncated to l.
	return hash[:min(len(hash), l)]
}

// truncate truncates s to l length by replacing the
// end of s with -suffix.
func truncate(l int, s, suffix string) string {
	if l >= len(s) {
		// under the limit, nothing to do
		return s
	}
	if l <= len(suffix) {
		// easy case, just return the start of the suffix
		return suffix[:min(l, len(suffix))]
	}
	return s[:l-len(suffix)-1] + "-" + suffix
}

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

func apiconfigsource(clusters ...string) *v2.ConfigSource {
	return &v2.ConfigSource{
		ConfigSourceSpecifier: &v2.ConfigSource_ApiConfigSource{
			ApiConfigSource: &v2.ApiConfigSource{
				ApiType:      v2.ApiConfigSource_GRPC,
				ClusterNames: clusters,
			},
		},
	}
}

func servicename(meta metav1.ObjectMeta, port string) string {
	return meta.Namespace + "/" + meta.Name + "/" + port
}
