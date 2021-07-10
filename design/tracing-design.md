# Design for Supporting Tracing in Contour

Status: Draft

## Abstract
Envoy has rich support for [distributed tracing][1].
However, Contour does not currently offer a way to configure Envoy for tracing.
This design document proposes an implementation of tracing support in Contour.

## Background
Envoy's [documentation on tracing][1] says:
> Distributed tracing allows developers to obtain visualizations of call flows in large service oriented architectures. It can be invaluable in understanding serialization, parallelism, and sources of latency.

Envoy supports:
- generating traces for incoming requests
- joining existing traces when a client sets the `x-client-trace-id` header
- exporting trace data to various third-party providers (Zipkin, Jaeger, Datadog, etc.)

The tracing ecosystem is complex right now.
However, a few observations can be made:
- [OpenTelemetry][2] is a CNCF project which is working to become a standard in the space. It was formed as a merger of the OpenTracing and OpenCensus projects. It supports ingesting trace data in a variety of formats, transforming it, and exporting it to a variety of backends.
- [Zipkin][3] and [Jaeger][4] are two other common providers. Jaeger supports collecting data in the Zipkin format as well as in its own format.

## Goals
- **Support a single global trace configuration** that applies to all requests.
- **Support OpenCensus and Zipkin trace formats**, since together they enable a large set of common backends to be used.

## Non Goals
- **Per-HTTPProxy trace configuration.** There is a technical limitation to enabling per-proxy configuration. Trace settings are configured on the HTTP connection manager (HCM). Each TLS-enabled virtual host uses its own HCM, but *all* non-TLS virtual hosts share a single HCM. As such, while it's possible to have unique trace settings for each TLS virtual host, it's not possible to do the same for non-TLS virtual hosts.  Note that this same limitation has come up when designing external authorization and global rate limiting as well.
- **Trace formats other than OpenCensus and Zipkin.** While Contour *may* choose to add direct support for other trace formats in the future, our hope is that by supporting the OpenTelemetry collector, users are able to export traces to the backend of their choice without having to directly configure it in Contour/Envoy.

## High-Level Design
The Contour configuration file will be extended with a new optional `tracing` section.
This configuration block, if present, will enable tracing and will define the trace format and other properties needed to generate and export trace data.
Initially, the `opencensus` and `zipkin` trace formats will be supported.

The parameters defined in the configuration file will be used to configure each [HTTP connection manager's `tracing` section][5].

Since the tracing backend is pluggable, Contour will not package any particular backend.
The user is responsible for deploying and operating the tracing backend of their choice, and configuring Contour to make use of it.

## Detailed Design
The `tracing` section of the Contour config file will look like:

(WIP, this is the rough idea)
```yaml
tracing:
  # Trace format to use. Valid options are opencensus and zipkin.
  format: [opencensus, zipkin]
  config:
    # If tracing.format == opencensus, then this block is expected
    # to be populated.
    opencensus:
      agent-address: otel-collector.default:55678
    # If tracing.format == zipkin, then this block is expected
    # to be populated.
    zipkin:
      collector-cluster: <Envoy cluster name>
      collector-endpoint: /api/v2/spans
      collector-endpoint-version: ...
```

In the presence of the `tracing` block above, all HTTP connection managers (HCM) will be configured for tracing.

A Zipkin-enabled HCM will include the following as an example:
```go
...
Tracing: &http.HttpConnectionManager_Tracing{
    Provider: &envoy_config_trace_v3.Tracing_Http{
        Name: "envoy.tracers.zipkin",
        ConfigType: &envoy_config_trace_v3.Tracing_Http_TypedConfig{
            TypedConfig: protobuf.MustMarshalAny(&envoy_config_trace_v3.ZipkinConfig{
                CollectorCluster:         "default/zipkin/9411/da39a3ee5e",
                CollectorEndpoint:        "/api/v2/spans",
                CollectorEndpointVersion: envoy_config_trace_v3.ZipkinConfig_HTTP_JSON,
            }),
        },
    },
},
...
```

An OpenCensus-enabled HCM will include the following as an example:
```go
...
Tracing: &http.HttpConnectionManager_Tracing{
    Provider: &envoy_config_trace_v3.Tracing_Http{
    Name: "envoy.tracers.opencensus",
    ConfigType: &envoy_config_trace_v3.Tracing_Http_TypedConfig{
        TypedConfig: protobuf.MustMarshalAny(&envoy_config_trace_v3.OpenCensusConfig{
            OcagentAddress:         "otel-collector.default:55678",
            OcagentExporterEnabled: true,
        }),
    },
},
...
```

For Zipkin, an Envoy cluster corresponding to the Zipkin collector must exist. There are a few different options for creating this cluster:
- user explicitly defines the cluster as part of a custom bootstrap file
- Contour config file takes a Kubernetes service name, and a DAG processor adds a cluster for it to the DAG as a root.
- Contour config file takes an address, and a DAG processor adds a cluster for it to the DAG as a root.

For OpenCensus, Envoy can only be configured once per lifetime (see [this Envoy docs section][6] for information). Attempts to modify the OpenCensus configuration will cause Envoy to NACK config updates which, because of https://github.com/projectcontour/contour/issues/1176, will cause all future config updates to be ignored until Envoy is restarted. 

## Alternatives Considered
If there are alternative high level or detailed designs that were not pursued they should be called out here with a brief explanation of why they were not pursued.

- additional providers (could be added down the road if needed, but desire is to standardize on OpenTelemetry eventually)

## Security Considerations
If this proposal has an impact to the security of the product, its users, or data stored or transmitted via the product, they must be addressed here.

## Compatibility
A discussion of any compatibility issues that need to be considered

- Migration from OpenCensus to OpenTelemery in the future

## Implementation
A description of the implementation, timelines, and any resources that have agreed to contribute.

## Open Issues
- is there any reason to use ExtensionService for this? (I think not; it's designed for the gRPC extension services and doesn't fit well here)
- OpenCensus can only be configured once per Envoy lifetime - so if a user changes the Contour config for OpenCensus and restarts Contour, Envoy will barf - how to deal with this?
- Zipkin requires an Envoy cluster to exist for the collector - how to get this into the DAG?

## Survey of Other Ingress Controllers

### Ambassador
https://www.getambassador.io/docs/edge-stack/latest/topics/running/services/tracing-service/
    - zipkin, lightstep, datadog

### Gloo
https://docs.solo.io/gloo-edge/latest/guides/observability/tracing/
    - anything Envoy supports    

[1]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/observability/tracing
[2]: https://opentelemetry.io/
[3]: https://zipkin.io/
[4]: https://www.jaegertracing.io/
[5]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-msg-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-tracing
[6]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-tracing
