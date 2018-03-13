# Contour 0.4

Heptio is pleased to announce the release of Contour 0.4.

I'd like to extend a warm thanks to [all of the contributors][6], [you're awesome][7].

## New and improved

In Contour 0.4 the JSON v1 bootstrap configuration option has be removed.
As it is now unused, the v1 JSON API has also been removed from the Contour codebase.
Consult the [upgrade notes][0] for how to update your Deployment or Daemonset manifests to the YAML bootstrap configuration format.

### Many general improvements made to the Contour -> Envoy transmission path

Much effort has been expended on the Contour to Envoy gRPC API, including a set of end to end tests to improve the robustness of configuration transfered to Envoy.

### Additional annotations

Contour now supports the following annotations to control Envoy's retry behaviour:
- `contour.heptio.com/request-timeout` to control the amount of time Envoy will wait for a backend to respond.
- `contour.heptio.com/retry-on` to control under which conditions Envoy should retry a request.
- `contour.heptio.com/num-retries` to control the number of retries Envoy will perform.
- `contour.heptio.com/per-try-timeout` to control the request timeout _per attempt_.
For more information please consult the [annotation documentation][4].
Thanks @cmaloney.
Fixes #164 and #221.

### Ingress class now configurable

By default Contour responds to the ingress class annotation `kubernetes.io/ingress.class: contour` where present.
However, if required while migrating from another ingress controller, you can pass the flag `--ingress-class-name` to adjust the `ingress.class` name that Contour will respond to.
Thanks @Nikkau.
Fixes #255

### TLS 1.1 is now mandatory

Following the [advice of the PCI Security Standards Council][5] Contour 0.4 will configure Envoy to require TLS 1.1 or later.
Thanks @sevein
Fixes #185

### Certificate information is now sent in-line in the gRPC message

Due to a limitation in Envoy Contour 0.3 exchanged certificate data with Envoy via a shared file system.
This limitation has been addressed and Contour 0.4 transmits certificate data directly to Envoy in gRPC API response messages. 
Thanks @sevein
Fixes #158

### Contour and Envoy can now live in separate pods

Although not currently utilised, Contour and Envoy can now exist in separate pods.
This is acomplished by the `--xds-address` and `--xds-port` flags passed to both `contour bootstrap` and `contour serve` which can be used to deploy Contour as a ReplicaSet which Envoy is a Daemonset. Thanks to @sevein. Fixes #165

### `ingress.kubernetes.io/force-ssl-redirect: "true"` annotation now applies on a per Ingress basis

Prior to Contour 0.4 if _any_ Ingress mentioned a virtual host used the `ingress.kubernetes.io/force-ssl-redirect: "true"` annotation, then all routes for that virtual host would be 301 upgraded.
In Contour 0.4 this behaviour is applied per Ingress, that is, all routes in an Ingress object.
This permits so-called _split_ Ingress setups where some routes on a virtual host will be 301 ugpraded, and others not. 
Fixes #250.

### Other bug fixes and improvements in this release

- The Contour docker image no longer bundles the `ocid` and `gcp` authentication plugins as they are not required for `-incluster` deployments.
- Daemonset or Deployment examples now specify the `--v2-config-only` flag to instruct Envoy to not fall back to parsing an invalid configuration file as the deprecated v1 JSON format. This should aid debugging `contour bootstrap` issues. Thanks @cmaloney
- Updated to the latest `envoyproxy/go-control-plane` library. Thanks @vaamarnath. Fixes #225
- Contour has switched to sirupsen/logrus as its logging library. Fixes #162.
- Our [troubleshooting][3] documentation now includes a simple way to access Envoy's Admin interface which is useful for examining the state of its route and cluster tables.
- Various issues related to updating Services and Ingresses in place via `kubectl edit` have been by the introduction of a new caching layer in the translator package. 

## Upgrading

Until Envoy 1.6 is released it is recommended to pin the version of Envoy used in your deployments to a known hash.
The recommended hash is
```
spec:
  containers:
  - image: docker.io/envoyproxy/envoy-alpine:e6ff690611b8a3373f6d66066b52d613873e446e
```
Consult the [upgrade notes][0] for how to update your Deployment or Daemonset manifests to the YAML bootstrap configuration format.

[0]: docs/upgrade.md
[1]: https://kubernetes.io/docs/concepts/services-networking/ingress/#tls
[2]: docs/tls.md
[3]: docs/troubleshooting.md
[4]: annotations.md
[5]: https://blog.pcisecuritystandards.org/are-you-ready-for-30-june-2018-sayin-goodbye-to-ssl-early-tls
[6]: https://github.com/heptio/contour/graphs/contributors
[7]: https://www.ephemera-inc.com/You-re-Awesome-p/6401.htm
