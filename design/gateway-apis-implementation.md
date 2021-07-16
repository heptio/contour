# Gateway API implementation design

Status: Accepted (Revised: June 2021)

## Abstract
The Gateway API is the evolution of the Kubernetes APIs that relate to `Services`, such as Ingress.
This document outlines what parts of these APIs Contour will implement, and how it will do so.

## Background
The Gateway API project is a subproject of Kubernetes SIG-Network, and is an attempt to re-do the mechanics around `Services` and `Ingress`, and how they interact.
It consists of a number of objects, not just the Gateway object, but that object is the key, so the API as a whole is named after it.
The project is aiming to cover the things currently implemented by Layer 4 load balancers that implement `Services` of type `LoadBalancer`, and ingress controllers (like Contour).

The Gateway API targets three personas:
- Infrastructure provider: The infrastructure provider (infra) is responsible for the overall environment that the cluster(s) are operating in. Examples include: the cloud provider (AWS, Azure, GCP, ...), the PaaS provider in a company.
- Cluster operator: The cluster operator (ops) is responsible for administration of entire clusters. They manage policies, network access, application permissions.
- Application developer: The application developer (dev) is responsible for defining their application configuration (e.g. timeouts, request matching/filter) and Service composition (e.g. path routing to backends).

The cluster operator and application developer are basically the same as Contour's Cluster Administrator and Application Developer personas, which will be important for this design.

In terms of the APIs themselves, the Gateway API has 3 primary API resources (taken from the gateway-apis docs site):

- GatewayClass defines a set of gateways with a common configuration and behavior.
- Gateway requests a point where traffic can be translated to Services within the cluster, using an internal Listener construct.
- Routes describe how traffic coming via the Gateway maps to the Services.

In Contour, we've previously solved a lot of the same problems with HTTPProxy (and IngressRoute before it).
That functionality can be described in the Gateway API by HTTPRoutes and TLSRoutes, as they describe Layer 7 ingress, as Contour does today.

Other types of routes include TCPRoutes and UDPRoutes, which are intended for Layer 4 load balancers.
Implementations may also define their own Route objects, at the cost of interoperability.

In terms of its stated goals, Contour is aiming at being an ingress controller - that is, a Layer 7 proxy with some api gateway functions.
Currently, Contour provides "TCP Proxying" that allows the forwarding of TLS streams based on the SNI information, which is precisely what the Gateway API TLSRoute object is for.
If Project Contour (the organisation) does add support for TCP and UDP forwarding, it will not be in the `projectcontour/contour` repo, but will be a separate repo.

This design is intended to cover the initial alpha releases of the Gateway API. 
We will aim to implement the core featureset of the APIs at that point.
We will then work with the upstream community on features that Contour and HTTPProxy currently support, but the Gateway API do not, and how best to represent those features in the APIs.
So, there are some features that are currently gaps for Contour (wildcard domain names, and exact path matching, for example),
and some that Contour supports that the Gateway API do not (websockets, configurable timeouts, header replacement, external auth, rate limiting, and so on).

Our eventual ideal is that HTTPProxy and Gateway API will have feature parity.
Practically, there may be times when HTTPProxy can move faster than Gateway API because we have a smaller feature surface.
We see this as a chance for HTTPProxy to test out ideas for Gateway API functionality, and provide feedback on what features should be in the core, extension, and implementation-specific parts of the API.

## Goals
- Define a data model and implementation for Contour's Gateway API support, covering the v1alpha1 version of the Gateway API.
- Layer 7 support only, which means HTTPRoutes, TLSRoutes only.

## Non Goals
- No TCPRoute or UDPRoute support, that is, no support for arbitrary TCP/UDP forwarding.

## High-Level Design

### Gateway API implementation surface area

Contour's support for the Gateway API will include support for the HTTPRoute and TLSRoute objects only, not any other type of Route - this includes no TCPRoute or UDPRoute support.
Contour is a layer 7 ingress controller, and the layer 4 load balancing implies by TCPRoute and UDPRoute is out of scope for this tool (that is, `projectcontour/contour`).
Project Contour (the organisation) may investigate a Gateway API based layer 4 solution in the future, but that effort will not be in `projectcontour/contour`.

### Contour Gateway model

Contour considers a Gateway to describe a single Envoy deployment.
Contour expects to be supplied with a `ControllerName` which is required to match a `GatewayClass`.
Contour will update the status of both the Gateway & GatewayClass.
When a `ControllerName` is not supplied, Contour will not do Gateway API processing.
Contour will merge Listeners within its Gateway. (See the detailed design for the exact rules Contour will use for this.)

### Dynamic Envoy Configuration

Typically, the fleet of Envoy's that connect to Contour to get their configuration information has been managed manually by the cluster operator.
Given a more dynamic configuration API, it's convenient to have the Envoy fleet configuration in Kubernetes be managed by a controller.
This allows cluster operator to simply deploy a Contour deployment, leaving the details about how Envoy is provisioned in the cluster as well as exposed outside of the cluster to a configuration file or CRD.

This behavior is meant to be optional and layered, such that users who want a fully managed environment can deploy the Contour Operator which manages instances of Contour which in turn manages instances of Envoy.
However, other users may not desire that level of controller driven infrastructure.
This design leaves users with their choice of deployment models. 

### Interrelated watches

The Gateway API is a set of interrelated Kubernetes objects, where a change in one object can mean that the scope of objects Contour is interested in will change.
Because of this, Contour will watch all events associated with its named GatewayClass, Gateway, HTTPRoute, TLSRoute, and BackendPolicy objects, and filter the objects it takes action on internally.

### Combining Gateway API with other configuration

When it calculates the Envoy configuration using the DAG, Contour layers different types of configuration in order.
Currently, Ingress is overwritten by HTTPProxy. 
If an Ingress and a HTTPProxy specify the exact same route, the HTTPProxy will win.

Once we have the Gateway API available as well, the processing order will be Ingress is overwritten by HTTPProxy, is overwritten by the Gateway API.
We could potentially reverse HTTPProxy and the Gateway API here, but this acknowledges that the Gateway API are really an evolution of the ideas in HTTPProxy, and will probably end up being the community standard.

### Status management

Each object in the Gateway API set has its own status definition. 
In general, Contour will update the status of any object that comes into scope with its details, ensuring that the `observedGeneration` field is set correctly.
When objects fall out of scope, any status set by Contour will not be removed.
It's expected that things that check the status will also check the `observedGeneration` to check if the status information is up-to-date with the current object generation.

### Per-object design notes

#### GatewayClass

Contour watches for GatewayClass resources which can, optionally, refer to a configuration object that will then be used for Contour configuration.
A GatewayClass can reference a configuration resource (Configmap or CRD) which describes how the fleet of Envoys should be provisioned in the cluster along with how they are exposed outside of the cluster.

If the GatewayClass doesn't reference any configuration resources, then a standard set of configuration is utilized much like the Contour QuickStart today.

Contour will need to be configured with the controller name matching the GatewayClass's `Spec.Controller` field.

#### Gateway
Contour will watch its configured Gateway object.

Contour is not able to specify the address that the Envoy deployment will listen on, and so will ignore any entries in the Gateway's `spec.addresses` field.
Contour will not update the `status.addresses` field.

For Contour, the key Gateway section are the Listeners.
These define how an implementation should listen for traffic described by Routes.

For Listeners:
- Listeners inside Gateways will be merged where possible.
- Conflicts within a Gateway will result in the relevant Listeners both being rejected (as there is no way to determine which one was first).
- Listeners are considered mergeable if all the fields out of `hostname`, `port`, and `protocol` match, with some additional rules around TLS.
- Further merging rules are specified in the detailed design below.
- Listeners that refer to any other Route than HTTPRoute or TLSRoute will be ignored, and a condition placed on the corresponding `status.listeners[]` object saying that it was ignored because those objects are not supported.

The Gateway may supply TLS config, in which case it is used as a default.
The TLS config may be overridden by HTTPRoutes if the Gateway `spec.listeners[].tls.routeOverride` is set to `Allow`.
This allows the Gateway to configure a default TLS certificate.
Note that this feature has fiddly interactions with the various places in which a hostname may be specified;
this will require careful test case design.

The output of this watcher is both Envoy configuration, as well as an optional set of Kubernetes resources automatically managed.

### HTTPRoute
Contour will watch all HTTPRoute objects, and filter for entries as per the spec.
This is a two level filter, by the configured Gateway's rules about namespaces, and then by the label selector for the Routes themselves.
Configuration of HTTPRoutes will be also subject to the rules around the `RouteGateways` field, for filtering which Gateways the HTTPRoute is allowed to be referenced by.

When a HTTPRoute specifies a hostname or slice of hostnames, those hostnames must match the hostnames in the Gateway.
Note that more specific precise matches at the Hostname level may match less specific wildcard matches at the Gateway level.
This allows the Gateway to define a default TLS certificate, which may only be overridden

The HTTPRoute also has a facility to supply additional TLS Config using the `tls` stanza (the RouteTLSConfig field).
The most important part here is that the field is only used if the `AllowRouteOverride` field is set in the referencing Gateway resource.

Contour will only ever update the `status` of HTTPRoute objects.

Errors or conflicts here will render that rule invalid, but not the rest of the rules.
Other valid rules will still be passed to Envoy.
For each invalid rule, Contour will update status information with the rule and the reason.
A conflict will **not** result in the whole HTTPRoute being rejected unless there are zero rules left.

The output of this watcher is Envoy configuration.

### TLSRoute
Contour will watch all TLSRoute objects, and filter for entries matching a label selector in its configured Gateway's `spec.listeners[].routes`,
and will configure the associated routes.
Configuration of TLSRoutes will be subject to the rules around the `RouteGateways` field.

Contour will only ever update the `status` of TLSRoute objects.

Errors or conflicts here will render that section of the config invalid.
Other valid sections will still be passed to Envoy.

The output of this watcher is Envoy configuration.

## Detailed Design

### Configuration
Contour will have an entry added to the config file for the `ControllerName` it should watch.

```yaml
gateway:
  controllerName: 
  name: gatewayname  # <--- deprecate
  namespace: gatewaynamespace  # <-- deprecate
```
Contour will look for a Gateway + GatewayClass that matches the `ControllerName` field specified in the configuration file.
If it does not find a matching Gateway/GatewayClass, then it will begin processing GatewayAPI resources once they are available. 

Should two different Gateway or GatewayClasses be found matching the same `ControllerName`, then Contour will utilize the oldest created object and set a status on the other(s) which describes that object as being rejected.

If the gateway is removed while Contour is operating, then an error will be logged, and all config associated with the Gateway API will be removed from Envoy.

_NOTE: The fields `name` & `namespace` will be deprecated from the configuration file._

#### Managed Envoy

Contour can optionally provision Envoy infrastructure in a Kubernetes cluster.
A new field will be added to the Contour configuration file which will determine if Contour should dynamically provision a fleet of Envoys with a corresponding service.
The field will be named `envoy` and contain a single value for now of `managed: true` or `managed: false`.

This field will default to `false` if unspecified.

Example config file:
```yaml
envoy:
  managed: true
gateway:
  controllerName: projectcontour.io/ingress-controller
```

_NOTE: Typically Envoy is deployed as a Daemonset, but not required (could also be a Deployment), so the term `fleet` is used to identify the set of Kubernetes managed pods which represent an Envoy instance._

##### managed: true

If envoy.managed is true, then Contour will automatically create a Envoy fleet/service.
Additionally, if a `Gateway.ControllerName` is set then the ports that are assigned to the Envoy service will match the Gateway discovered based upon the `ControllerName`.

Additional RBAC permissions will be required to the namespace where Contour is deployed to allow it to dynamically manage Daemonsets/Deployments/Services, etc.

##### managed: false

If envoy.managed is false, then Contour won't manage the Envoy fleet/service.
It's up to the cluster operator to deploy and managed these resources.

If `Gateway.ControllerName` is not set, then the `envoy-service-xxx` flags passed to Contour will be used to define the ports which Envoy/Service will utilize. 

_See: https://github.com/projectcontour/contour/issues/3616_

### GatewayClass ParametersRef 

The `GatewayClass.parametersRef` field is a reference to an implementation-specific resource containing configuration parameters associated to the GatewayClass.
This field will be used to expose Envoy-specific configuration.
If the field is unspecified and the config file contains `envoy.managed: true`, then Contour will create an instance of Envoy with default settings.
Since `parametersRef` does not contain a namespace field, the referenced resource must exist in a known namespace.
The same namespace where Contour is deployed will be used as the known namespace.
If the referenced resource cannot be found, Contour will set the GatewayClass InvalidParameters status condition to true.

This method of baking all the configuration of each specific provider is how the operator planned on implementing external configurations of the Envoy service, meaning
each `Service.Type: LoadBalancer` needs specific configuration parameters defined so that the service can be properly managed if Contour in responsible.

The end goal is to remove the use of `Service.Type: LoadBalancer` and utilize Gateways for the same.
So instead of having Contour need to understand each provider type and also require PRs+code to support changes and new providers, Contour could utilize Gateways for all external interactions.

This would be that instead of having a `parametersRef` field that references a CRD (or configmap) which defines how a Cloud Load Balancer should be configured (and in code the corresponding annotations),
Contour could create a second Gateway which selects the Envoy pods and is responsible for configuring the load balancer + traffic.
Reference Issue: https://github.com/projectcontour/contour/issues/3702

At the time of writing this design, however, many providers do not yet have GatewayAPI implementations.
We could stub out this work and essentially move the code that would be baked into Contour into a separate project.
Users can utilize this project until providers catch up with their implementations. 
At that time, the separate project would go away and be deprecated. 

Some work on this has been started: https://github.com/stevesloka/gateway-lb

#### Hostname matching

In many of the Gateway API objects, Hostnames *may* be specified, and *may* be either "precise" (a domain name without the terminating dot of a network host), or "wildcard" (a domain name prefixed with a single wildcard label).
Per the API spec, only the first DNS label of the hostname may be a wildcard, and the wildcard must match only a single label.

Hostnames are considered to match if they exactly match, or if a precise hostname matches the suffix of a wildcard hostname (that is, if they match after the first DNS label is discarded.)

This DNS matching is referred to in this proposal as a "domain match", as opposed to an "exact match", which is string equality only.

### Gateway

When ingesting Gateways, Contour will import the configured named Gateway into its cache, and watch it for spec changes. Spec changes will trigger a DAG run.

#### Listener Ports

Contour allows the configuration of secure and insecure ports, but the ports specified in the Listener must match the ports as reachable from outside the cluster.

In the example deployment, the port that is configured for the listener is the port as it is configured as a hostPort on the Envoy deployment.
The hostPort is responsible for getting the insecure traffic (bound for port `80`) to the actual insecure listener (listening on port `8080`), and similarly for the secure traffic from `443` to `8443`.

If you change the fields insecure and secure traffic is expected to go to from outside away from `80` and `443` respectively, we need to have a way to tell the redirect generator what to do, and a way to determine if the ports that a Gateway Listener is allowed to request.

In either of these cases, the redirect generator can use the value of the secure port to generate redirects correctly.
So if you're using `443` externally, requests to `http://foo.com` redirect will go to `https://foo.com`.
If using any other port for the secure port, requests to `http://foo.com` redirect will go to `https://foo.com:<port>` instead.

We considered (in [#3263](https://github.com/projectcontour/contour/pull/3263)) changing Contour to be able to add extra listeners as well as the secure and insecure ones, but decided it was additional complexity this implementation did not initially need.

#### Listener merging

Listener merging is performed by Contour to coalesce any set of valid Listeners into minimal configuration for Envoy.
Listener **conflict** is defined as two listeners that are not mergeable for some reason.

Listeners that are mergeable but have a conflict are both invalid and will be rejected, and their status updated accordingly.

This behavior is similar to the HTTPProxy processor's `validHTTPProxies()` method.

#### Listener merging rules

In general, Listeners that match on ProtocolType and PortNumber can be merged, using Hostname as a discriminator.

Contour uses the following general rules for merging Listeners.

1. Contour's rules about port numbers and secure and insecure ports must be satisfied.
1. A group is considered to be any set of Listeners that have the same values for the `ProtocolType` and `PortNumber` fields.
1. Either each Listener within the group specifies the “HTTP” Protocol or each Listener within the group specifies either the “HTTPS” or “TLS” Protocol.
1. Each Listener within the group specifies a Hostname that does not exactly match any other hostname within the group. Note that HTTPRoutes may *also* specify Hostnames, which may be more specific, and which will be matched with domain matching to the one specified in their Listener.
1. As a special case, one Listener within a group may omit Hostname, in which case this Listener matches when no other Listener matches.

Listeners that match on ProtocolType, PortNumber, and exactly match Hostname must also have matching TLS details (GatewayTLSConfig struct) to be merged.
Listeners that have different TLS config but the same other details are in conflict.

Listeners that are not mergeable may refer to the same TLS object.
Contour does not check the SAN of any referred certificates.

Listeners that match on ProtocolType, PortNumber, and Hostname, but have different GatewayTLSConfig structs (that is, the `tls` stanza is different) are in conflict.

Routes for a Listener are chosen using the `RoutBindingSelector`, as per the spec.
Precedence rules in the spec must also be followed.

The rules for implementing RouteBindingSelector are straightforward and will be implemented per the spec.

#### Gateway Status

Contour will update all status fields in the Gateway.
`ListenerStatus` entries are expected to be keyed by `port`.
If more than one Listener shares the same port, the `ListenerStatus` reports the combined status.

As the API currently stands, Contour must combine the statuses for all Listeners that share a `port`, including Listeners that are being merged.

Whenever Contour looks at a Listener, it will add an `Admitted` Condition.
In the event that a ListenerStatus has everything okay (all mergeable Listeners pass validation), the `Admitted` condition will be `status: true`.
If there are any errors, the `Admitted` Condition will be `status: false`, and the Reason will tell you more about why.

### *Route general notes

For both HTTPRoute and TLSRoute, there are two fields in common, `spec.gateways`, and `status.gateways`.
Both resources will handle these fields as per the spec.

The thing that's most worthy of comment is that if a Route is selected by a Listener's RouteBindingSelector, but does _not_ allow the Gateway in its RouteGateways, then the Route's `status.gateways[].conditions` field will have an `Admitted` Condition with `status: false` as per the spec.

### HTTPRoute

#### RouteTLSConfig

For HTTPRoute, the most complex part for implementation is the RouteTLSConfig struct, which allows the definition of TLS certificates for use with the associated Hostname.
This can only be used if the `spec.listeners[].tls.routeOverride` field in the referencing Gateway resource is `Allow`.

We will need to ensure we have a good set of tests around how this field and the Gateway TLSConfig interact.

Testing is needed to cover the following variables:
- Gateway has TLS config true/false
- Gateway allows TLS override true/false
- Route has TLS override true/false
- Route and Gateway match hostnames true/false

In addition, we must ensure that the TLS config in the route will override the TLS config at the Gateway if it's allowed even if there are multiple matches (for example, if HTTPRoutes share the same hostname).

#### Conflict Resolution and Route merging

HTTPRoutes may be merged into a single Envoy config by Contour as long as:
- the Hostnames match exactly and the TLS Config matches
- the HTTPRouteRules are different

In the event that two HTTPRoutes match on Hostname and RouteTLSConfig, and have matching HTTPRouteRules (`rules` stanza), then the oldest HTTPRoute's HTTPRouteRules should win.
The HTTPRoute not accepted must have its `Admitted` Condition changed to `status: false` with a reason field to explain why, possibly including the name of the other resource.

If Hostnames match exactly, but TLS Config does not, that is a conflict and the oldest-object-wins rule applies also.
That is, you can't specify a different certificate for the same hostname.

### TLSRoute

TLSRoute implementation is quite straightforward, only allows simple routing based on SNI.
Contour will implement this object as per its API spec.

### Other concerns

## Alternatives Considered

### Contour watches multiple Gateways
The only main alternative considered for this design was making Contour possibly responsible for more than one Gateway.
In this model, Contour would be configured with a `controller` string, and watch all GatewayClasses for that string in the `spec.controller` field.
Contour would also watch all Gateways, and for Gateways with a matching `spec.gatewayClassName`, would use those as a basis for looking for routes.

The reason this model wasn't chosen was that it's difficult to define how you could merge multiple Gateway objects into a single Envoy installation.
Model-wise, it seems to be intended that a Gateway represents the actual thing that takes traffic and transforms it so it can get to the requested Pods.
In Contour's case, this is Envoy, and it's extremely difficult to design a model where a single Envoy deployment could handle multiple different Gateway specs.
