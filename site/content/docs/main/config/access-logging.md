# Access Logging

## Overview

Contour allows you to control Envoy's access logging.
By default, HTTP and HTTPS access logs are written to `/dev/stdout` by the Envoy containers and look like following:

```
[2021-04-14T16:36:00.361Z] "GET /foo HTTP/1.1" 200 - 0 463 6 3 "-" "HTTPie/1.0.3" "837aa8dc-344f-4faa-b7d5-c9cce1028519" "localhost:8080" "127.0.0.1:8081"
```

The detailed description of each field can be found in [Envoy access logging documentation][7].


## Customizing Access Log Destination and Formats

You can change the destination file where the access log is written by using Contour [command line parameters][1] `--envoy-http-access-log` and `--envoy-https-access-log`.

The access log can take two different formats, both can be customized

* Text based access logs, like shown in the example above.
* Structured JSON logging.

### Text Based Access Logging

Ensure that you have selected `envoy` as the access log format.
Note that this is the default format if the parameters are not given.

- Add `--accesslog-format=envoy` to your Contour startup line, or
- Add `accesslog-format: envoy` to your configuration file.

Customize the access log format by defining `accesslog-format-string` in your configuration file.

```yaml
accesslog-format-string: "[%START_TIME%] \"%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %PROTOCOL%\" %RESPONSE_CODE% %RESPONSE_FLAGS% %BYTES_RECEIVED% %BYTES_SENT% %DURATION% %RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)% \"%REQ(X-FORWARDED-FOR)%\" \"%REQ(USER-AGENT)%\" \"%REQ(X-REQUEST-ID)%\" \"%REQ(:AUTHORITY)%\" \"%UPSTREAM_HOST%\"\n"
```
After restarting Contour and successful validation of the configuration, the new format will take effect in a short while.

Refer to [Envoy access logging documentation][7] for the description of the command operators, and note that the format string needs to end in a linefeed `\n`.

### Structured JSON Logging

Contour allows you to choose from a set of JSON fields that will be expanded into Envoy templates and sent to Envoy.
There is a default set of fields if you enable JSON logging, and you may customize which fields you log.

The list of available fields are discoverable in the following objects:
- [JSONFields][2] are fields that have built in mappings to commonly used envoy operators.
- [envoySimpleOperators][3] are the names of simple envoy operators that don't require arguments, they are case-insensitive when configured.
- [envoyComplexOperators][4] are the names of complex envoy operators that require arguments.

The default list of fields is available at [DefaultFields][5].

#### Enabling the Feature

To enable the feature you have two options:

- Add `--accesslog-format=json` to your Contour startup line.
- Add `accesslog-format: json` to your configuration file.

Without any further customization, the [default fields][5] will be used.

#### Customizing Logged Fields

To customize the logged fields, add a `json-fields` list of strings to your configuration file.
If the `json-fields` key is not specified, the [default fields][5] will be configured.

To use a value from [JSONFields][2] or [envoySimpleOperators][3], simply include the name of the value in the list of strings.
The JSONFields are case-sensitive, but envoySimpleOperators are not.

To use [envoyComplexOperators][4] or to use alternative field names, specify strings as key/value pairs like `"fieldName=%OPERATOR(...)%"`.

Unknown field names in non key/value fields will result in validation errors, as will unknown Envoy operators in key/value fields.
Note that the `DYNAMIC_METADATA` and `FILTER_STATE` Envoy logging operators are not supported at this time due to the complexity of their validation.

See the [example config file][6] to see this used in context.

#### Sample Configuration File

Here is a sample config:

```yaml
accesslog-format: json
json-fields:
  - "@timestamp"
  - "authority"
  - "bytes_received"
  - "bytes_sent"
  - "customer_id=%REQ(X-CUSTOMER-ID)%"
  - "downstream_local_address"
  - "downstream_remote_address"
  - "duration"
  - "method"
  - "path"
  - "protocol"
  - "request_id"
  - "requested_server_name"
  - "response_code"
  - "response_flags"
  - "uber_trace_id"
  - "upstream_cluster"
  - "upstream_host"
  - "upstream_local_address"
  - "upstream_service_time"
  - "user_agent"
  - "x_forwarded_for"
```

## Using Access Log Formatter Extensions

Envoy allows implementing custom access log command operators as extensions.
Following extensions are supported by Contour:

| Command operator | Description |
|------------------|-------------|
| [REQ_WITHOUT_QUERY][8] | Works the same way as REQ except that it will remove the query string. It is used to avoid logging any sensitive information into the access log. |



[1]: ../configuration#serve-flags
[2]: https://github.com/projectcontour/contour/blob/main/pkg/config/accesslog.go#L33-L45
[3]: https://github.com/projectcontour/contour/blob/main/pkg/config/accesslog.go#L49-L93
[4]: https://github.com/projectcontour/contour/blob/main/pkg/config/accesslog.go#L97-L102
[5]: https://github.com/projectcontour/contour/blob/main/pkg/config/accesslog.go#L4
[6]: {{< param github_url >}}/tree/{{< param latest_version >}}/examples/contour/01-contour-config.yaml
[7]: https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/access_log/usage
[8]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/formatter/req_without_query/v3/req_without_query.proto
