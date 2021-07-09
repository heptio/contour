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
	"errors"
	"fmt"
	"sync"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/k8s"
	ingress_validation "github.com/projectcontour/contour/internal/validation/ingress"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// A KubernetesCache holds Kubernetes objects and associated configuration and produces
// DAG values.
type KubernetesCache struct {
	// RootNamespaces specifies the namespaces where root
	// HTTPProxies can be defined. If empty, roots can be defined in any
	// namespace.
	RootNamespaces []string

	// Contour's IngressClassName.
	// If not set, defaults to DEFAULT_INGRESS_CLASS.
	IngressClassName string

	// Secrets that are referred from the configuration file.
	ConfiguredSecretRefs []*types.NamespacedName

	ingresses                 map[types.NamespacedName]*networking_v1.Ingress
	ingressclass              *networking_v1.IngressClass
	httpproxies               map[types.NamespacedName]*contour_api_v1.HTTPProxy
	secrets                   map[types.NamespacedName]*v1.Secret
	tlscertificatedelegations map[types.NamespacedName]*contour_api_v1.TLSCertificateDelegation
	services                  map[types.NamespacedName]*v1.Service
	namespaces                map[string]*v1.Namespace
	gatewayclass              *gatewayapi_v1alpha1.GatewayClass
	gateway                   *gatewayapi_v1alpha1.Gateway
	httproutes                map[types.NamespacedName]*gatewayapi_v1alpha1.HTTPRoute
	tlsroutes                 map[types.NamespacedName]*gatewayapi_v1alpha1.TLSRoute
	tcproutes                 map[types.NamespacedName]*gatewayapi_v1alpha1.TCPRoute
	udproutes                 map[types.NamespacedName]*gatewayapi_v1alpha1.UDPRoute
	backendpolicies           map[types.NamespacedName]*gatewayapi_v1alpha1.BackendPolicy
	extensions                map[types.NamespacedName]*contour_api_v1alpha1.ExtensionService

	initialize sync.Once

	logrus.FieldLogger
}

// init creates the internal cache storage. It is called implicitly from the public API.
func (kc *KubernetesCache) init() {
	kc.ingresses = make(map[types.NamespacedName]*networking_v1.Ingress)
	kc.httpproxies = make(map[types.NamespacedName]*contour_api_v1.HTTPProxy)
	kc.secrets = make(map[types.NamespacedName]*v1.Secret)
	kc.tlscertificatedelegations = make(map[types.NamespacedName]*contour_api_v1.TLSCertificateDelegation)
	kc.services = make(map[types.NamespacedName]*v1.Service)
	kc.namespaces = make(map[string]*v1.Namespace)
	kc.httproutes = make(map[types.NamespacedName]*gatewayapi_v1alpha1.HTTPRoute)
	kc.tcproutes = make(map[types.NamespacedName]*gatewayapi_v1alpha1.TCPRoute)
	kc.udproutes = make(map[types.NamespacedName]*gatewayapi_v1alpha1.UDPRoute)
	kc.tlsroutes = make(map[types.NamespacedName]*gatewayapi_v1alpha1.TLSRoute)
	kc.backendpolicies = make(map[types.NamespacedName]*gatewayapi_v1alpha1.BackendPolicy)
	kc.extensions = make(map[types.NamespacedName]*contour_api_v1alpha1.ExtensionService)
}

// matchesIngressClass returns true if the given IngressClass
// is the one this cache is using.
func (kc *KubernetesCache) matchesIngressClass(obj *networking_v1.IngressClass) bool {
	// If no ingress class name set, we allow an ingress class that is named
	// with the default Contour accepted name.
	if kc.IngressClassName == "" {
		return obj.Name == ingress_validation.DefaultClassName
	}
	// Otherwise, the name of the ingress class must match what has been
	// configured.
	if obj.Name == kc.IngressClassName {
		return true
	}
	return false
}

// ingressMatchesIngressClass returns true if the given Ingress object matches
// the configured ingress class name via annotation or Spec.IngressClassName
// and emits a log message if there is no match.
func (kc *KubernetesCache) ingressMatchesIngressClass(obj *networking_v1.Ingress) bool {
	if !ingress_validation.MatchesIngressClassName(obj, kc.IngressClassName) {
		// We didn't get a match so report this object is being ignored.
		kc.WithField("name", obj.GetName()).
			WithField("namespace", obj.GetNamespace()).
			WithField("kind", k8s.KindOf(obj)).
			WithField("ingress-class-annotation", annotation.IngressClass(obj)).
			WithField("ingress-class-name", pointer.StringPtrDerefOr(obj.Spec.IngressClassName, "")).
			WithField("target-ingress-class", kc.IngressClassName).
			Debug("ignoring object with unmatched ingress class")
		return false
	}
	return true
}

// matchesIngressClassAnnotation returns true if the given Kubernetes object
// belongs to the Ingress class that this cache is using.
func (kc *KubernetesCache) matchesIngressClassAnnotation(obj metav1.Object) bool {
	if !annotation.MatchesIngressClass(obj, kc.IngressClassName) {
		kc.WithField("name", obj.GetName()).
			WithField("namespace", obj.GetNamespace()).
			WithField("kind", k8s.KindOf(obj)).
			WithField("ingress-class", annotation.IngressClass(obj)).
			WithField("target-ingress-class", kc.IngressClassName).
			Debug("ignoring object with unmatched ingress class")
		return false
	}

	return true
}

// Insert inserts obj into the KubernetesCache.
// Insert returns true if the cache accepted the object, or false if the value
// is not interesting to the cache. If an object with a matching type, name,
// and namespace exists, it will be overwritten.
func (kc *KubernetesCache) Insert(obj interface{}) bool {
	kc.initialize.Do(kc.init)

	if obj, ok := obj.(metav1.Object); ok {
		kind := k8s.KindOf(obj)
		for key := range obj.GetAnnotations() {
			// Emit a warning if this is a known annotation that has
			// been applied to an invalid object kind. Note that we
			// only warn for known annotations because we want to
			// allow users to add arbitrary orthogonal annotations
			// to objects that we inspect.
			if annotation.IsKnown(key) && !annotation.ValidForKind(kind, key) {
				// TODO(jpeach): this should be exposed
				// to the user as a status condition.
				kc.WithField("name", obj.GetName()).
					WithField("namespace", obj.GetNamespace()).
					WithField("kind", kind).
					WithField("version", k8s.VersionOf(obj)).
					WithField("annotation", key).
					Error("ignoring invalid or unsupported annotation")
			}
		}
	}

	switch obj := obj.(type) {
	case *v1.Secret:
		valid, err := isValidSecret(obj)
		if !valid {
			if err != nil {
				kc.WithField("name", obj.GetName()).
					WithField("namespace", obj.GetNamespace()).
					WithField("kind", "Secret").
					WithField("version", k8s.VersionOf(obj)).
					Error(err)
			}
			return false
		}

		kc.secrets[k8s.NamespacedNameOf(obj)] = obj
		return kc.secretTriggersRebuild(obj)
	case *v1.Service:
		kc.services[k8s.NamespacedNameOf(obj)] = obj
		return kc.serviceTriggersRebuild(obj)
	case *v1.Namespace:
		kc.namespaces[obj.Name] = obj
		return true
	case *networking_v1.Ingress:
		if kc.ingressMatchesIngressClass(obj) {
			kc.ingresses[k8s.NamespacedNameOf(obj)] = obj
			return true
		}
	case *networking_v1.IngressClass:
		if kc.matchesIngressClass(obj) {
			kc.ingressclass = obj
			return true
		}
	case *contour_api_v1.HTTPProxy:
		if kc.matchesIngressClassAnnotation(obj) {
			kc.httpproxies[k8s.NamespacedNameOf(obj)] = obj
			return true
		}
	case *contour_api_v1.TLSCertificateDelegation:
		kc.tlscertificatedelegations[k8s.NamespacedNameOf(obj)] = obj
		return true
	case *gatewayapi_v1alpha1.GatewayClass:
		kc.gatewayclass = obj
		return true
	case *gatewayapi_v1alpha1.Gateway:
		kc.gateway = obj
		return true
	case *gatewayapi_v1alpha1.HTTPRoute:
		kc.httproutes[k8s.NamespacedNameOf(obj)] = obj
		return true
	case *gatewayapi_v1alpha1.TCPRoute:
		kc.tcproutes[k8s.NamespacedNameOf(obj)] = obj
		return true
	case *gatewayapi_v1alpha1.UDPRoute:
		kc.udproutes[k8s.NamespacedNameOf(obj)] = obj
		return true
	case *gatewayapi_v1alpha1.TLSRoute:
		kc.tlsroutes[k8s.NamespacedNameOf(obj)] = obj
		return true
	case *gatewayapi_v1alpha1.BackendPolicy:
		m := k8s.NamespacedNameOf(obj)
		// TODO(youngnick): Remove this once gateway-api actually have behavior
		// other than being added to the cache.
		kc.WithField("experimental", "gateway-api").WithField("name", m.Name).WithField("namespace", m.Namespace).Debug("Adding BackendPolicy")
		kc.backendpolicies[k8s.NamespacedNameOf(obj)] = obj
		return true
	case *contour_api_v1alpha1.ExtensionService:
		kc.extensions[k8s.NamespacedNameOf(obj)] = obj
		return true

	default:
		// not an interesting object
		kc.WithField("object", obj).Error("insert unknown object")
		return false
	}

	return false
}

// Remove removes obj from the KubernetesCache.
// Remove returns a boolean indicating if the cache changed after the remove operation.
func (kc *KubernetesCache) Remove(obj interface{}) bool {
	kc.initialize.Do(kc.init)

	switch obj := obj.(type) {
	default:
		return kc.remove(obj)
	case cache.DeletedFinalStateUnknown:
		return kc.Remove(obj.Obj) // recurse into ourselves with the tombstoned value
	}
}

func (kc *KubernetesCache) remove(obj interface{}) bool {
	switch obj := obj.(type) {
	case *v1.Secret:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.secrets[m]
		delete(kc.secrets, m)
		return ok
	case *v1.Service:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.services[m]
		delete(kc.services, m)
		return ok
	case *v1.Namespace:
		_, ok := kc.namespaces[obj.Name]
		delete(kc.namespaces, obj.Name)
		return ok
	case *networking_v1.Ingress:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.ingresses[m]
		delete(kc.ingresses, m)
		return ok
	case *networking_v1.IngressClass:
		if kc.matchesIngressClass(obj) {
			kc.ingressclass = nil
			return true
		}
		return false
	case *contour_api_v1.HTTPProxy:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.httpproxies[m]
		delete(kc.httpproxies, m)
		return ok
	case *contour_api_v1.TLSCertificateDelegation:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.tlscertificatedelegations[m]
		delete(kc.tlscertificatedelegations, m)
		return ok
	case *gatewayapi_v1alpha1.GatewayClass:
		kc.gatewayclass = nil
		return true
	case *gatewayapi_v1alpha1.Gateway:
		kc.gateway = nil
		return true
	case *gatewayapi_v1alpha1.HTTPRoute:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.httproutes[m]
		delete(kc.httproutes, m)
		return ok
	case *gatewayapi_v1alpha1.TCPRoute:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.tcproutes[m]
		delete(kc.tcproutes, m)
		return ok
	case *gatewayapi_v1alpha1.UDPRoute:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.udproutes[m]
		delete(kc.udproutes, m)
		return ok
	case *gatewayapi_v1alpha1.TLSRoute:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.tlsroutes[m]
		delete(kc.tlsroutes, m)
		return ok
	case *gatewayapi_v1alpha1.BackendPolicy:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.backendpolicies[m]
		// TODO(youngnick): Remove this once gateway-api actually have behavior
		// other than being removed from the cache.
		kc.WithField("experimental", "gateway-api").WithField("name", m.Name).WithField("namespace", m.Namespace).Debug("Removing BackendPolicy")
		delete(kc.backendpolicies, m)
		return ok
	case *contour_api_v1alpha1.ExtensionService:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.extensions[m]
		delete(kc.extensions, m)
		return ok

	default:
		// not interesting
		kc.WithField("object", obj).Error("remove unknown object")
		return false
	}
}

// serviceTriggersRebuild returns true if this service is referenced
// by an Ingress or HTTPProxy in this cache.
func (kc *KubernetesCache) serviceTriggersRebuild(service *v1.Service) bool {
	for _, ingress := range kc.ingresses {
		if ingress.Namespace != service.Namespace {
			continue
		}
		if backend := ingress.Spec.DefaultBackend; backend != nil {
			if backend.Service.Name == service.Name {
				return true
			}
		}

		for _, rule := range ingress.Spec.Rules {
			http := rule.IngressRuleValue.HTTP
			if http == nil {
				continue
			}
			for _, path := range http.Paths {
				if path.Backend.Service.Name == service.Name {
					return true
				}
			}
		}
	}

	for _, proxy := range kc.httpproxies {
		if proxy.Namespace != service.Namespace {
			continue
		}
		for _, route := range proxy.Spec.Routes {
			for _, s := range route.Services {
				if s.Name == service.Name {
					return true
				}
			}
		}
		if tcpproxy := proxy.Spec.TCPProxy; tcpproxy != nil {
			for _, s := range tcpproxy.Services {
				if s.Name == service.Name {
					return true
				}
			}
		}
	}

	for _, route := range kc.httproutes {
		if route.Namespace != service.Namespace {
			continue
		}
		for _, rule := range route.Spec.Rules {
			for _, forward := range rule.ForwardTo {
				if forward.ServiceName != nil {
					if *forward.ServiceName == service.Name {
						return true
					}
				}
			}
		}
	}

	return false
}

// secretTriggersRebuild returns true if this secret is referenced by an Ingress
// or HTTPProxy object, or by the configuration file. If the secret is not in the same namespace
// it must be mentioned by a TLSCertificateDelegation.
func (kc *KubernetesCache) secretTriggersRebuild(secret *v1.Secret) bool {
	if _, isCA := secret.Data[CACertificateKey]; isCA {
		// locating a secret validation usage involves traversing each
		// proxy object, determining if there is a valid delegation,
		// and if the reference the secret as a certificate. The DAG already
		// does this so don't reproduce the logic and just assume for the moment
		// that any change to a CA secret will trigger a rebuild.
		return true
	}

	delegations := make(map[string]bool) // targetnamespace/secretname to bool

	// TODO(youngnick): Check if this is required.
	for _, d := range kc.tlscertificatedelegations {
		for _, cd := range d.Spec.Delegations {
			for _, n := range cd.TargetNamespaces {
				delegations[n+"/"+cd.SecretName] = true
			}
		}
	}

	for _, ingress := range kc.ingresses {
		if ingress.Namespace == secret.Namespace {
			for _, tls := range ingress.Spec.TLS {
				if tls.SecretName == secret.Name {
					return true
				}
			}
		}
		if delegations[ingress.Namespace+"/"+secret.Name] {
			for _, tls := range ingress.Spec.TLS {
				if tls.SecretName == secret.Namespace+"/"+secret.Name {
					return true
				}
			}
		}

		if delegations["*/"+secret.Name] {
			for _, tls := range ingress.Spec.TLS {
				if tls.SecretName == secret.Namespace+"/"+secret.Name {
					return true
				}
			}
		}
	}

	for _, proxy := range kc.httpproxies {
		vh := proxy.Spec.VirtualHost
		if vh == nil {
			// not a root ingress
			continue
		}
		tls := vh.TLS
		if tls == nil {
			// no tls spec
			continue
		}

		if proxy.Namespace == secret.Namespace && tls.SecretName == secret.Name {
			return true
		}
		if delegations[proxy.Namespace+"/"+secret.Name] {
			if tls.SecretName == secret.Namespace+"/"+secret.Name {
				return true
			}
		}
		if delegations["*/"+secret.Name] {
			if tls.SecretName == secret.Namespace+"/"+secret.Name {
				return true
			}
		}
	}

	// Secrets referred by the configuration file shall also trigger rebuild.
	for _, s := range kc.ConfiguredSecretRefs {
		if s.Namespace == secret.Namespace && s.Name == secret.Name {
			return true
		}
	}

	if kc.gateway != nil {
		for _, listener := range kc.gateway.Spec.Listeners {
			if listener.TLS == nil {
				continue
			}
			if listener.TLS.CertificateRef == nil {
				continue
			}

			ref := listener.TLS.CertificateRef
			if ref.Kind == "Secret" && ref.Group == "core" {
				if kc.gateway.Namespace == secret.Namespace && ref.Name == secret.Name {
					return true
				}
			}
		}
	}

	return false
}

// LookupSecret returns a Secret if present or nil if the underlying kubernetes
// secret fails validation or is missing.
func (kc *KubernetesCache) LookupSecret(name types.NamespacedName, validate func(*v1.Secret) error) (*Secret, error) {
	sec, ok := kc.secrets[name]
	if !ok {
		return nil, fmt.Errorf("Secret not found")
	}

	if err := validate(sec); err != nil {
		return nil, err
	}

	s := &Secret{
		Object: sec,
	}

	return s, nil
}

func (kc *KubernetesCache) LookupUpstreamValidation(uv *contour_api_v1.UpstreamValidation, namespace string) (*PeerValidationContext, error) {
	if uv == nil {
		// no upstream validation requested, nothing to do
		return nil, nil
	}

	secretName := types.NamespacedName{Name: uv.CACertificate, Namespace: namespace}
	cacert, err := kc.LookupSecret(secretName, validCA)
	if err != nil {
		// UpstreamValidation is requested, but cert is missing or not configured
		return nil, fmt.Errorf("invalid CA Secret %q: %s", secretName, err)
	}

	if uv.SubjectName == "" {
		// UpstreamValidation is requested, but SAN is not provided
		return nil, errors.New("missing subject alternative name")
	}

	return &PeerValidationContext{
		CACertificate: cacert,
		SubjectName:   uv.SubjectName,
	}, nil
}

// DelegationPermitted returns true if the referenced secret has been delegated
// to the namespace where the ingress object is located.
func (kc *KubernetesCache) DelegationPermitted(secret types.NamespacedName, targetNamespace string) bool {
	contains := func(haystack []string, needle string) bool {
		if len(haystack) == 1 && haystack[0] == "*" {
			return true
		}
		for _, h := range haystack {
			if h == needle {
				return true
			}
		}
		return false
	}

	if secret.Namespace == targetNamespace {
		// secret is in the same namespace as target
		return true
	}

	for _, d := range kc.tlscertificatedelegations {
		if d.Namespace != secret.Namespace {
			continue
		}
		for _, d := range d.Spec.Delegations {
			if contains(d.TargetNamespaces, targetNamespace) {
				if secret.Name == d.SecretName {
					return true
				}
			}
		}
	}
	return false
}

func validCA(s *v1.Secret) error {
	if len(s.Data[CACertificateKey]) == 0 {
		return fmt.Errorf("empty %q key", CACertificateKey)
	}

	return nil
}

// LookupService returns the Kubernetes service and port matching the provided parameters,
// or an error if a match can't be found.
func (kc *KubernetesCache) LookupService(meta types.NamespacedName, port intstr.IntOrString) (*v1.Service, v1.ServicePort, error) {
	svc, ok := kc.services[meta]
	if !ok {
		return nil, v1.ServicePort{}, fmt.Errorf("service %q not found", meta)
	}

	for i := range svc.Spec.Ports {
		p := svc.Spec.Ports[i]
		if int(p.Port) == port.IntValue() || port.String() == p.Name {
			switch p.Protocol {
			case "", v1.ProtocolTCP:
				return svc, p, nil
			default:
				return nil, v1.ServicePort{}, fmt.Errorf("unsupported service protocol %q", p.Protocol)
			}
		}
	}

	return nil, v1.ServicePort{}, fmt.Errorf("port %q on service %q not matched", port.String(), meta)
}
