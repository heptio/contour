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
	envoy_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_file_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoy_req_without_query_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/req_without_query/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/pkg/config"
)

// FileAccessLogEnvoy returns a new file based access log filter
func FileAccessLogEnvoy(path string, format string, extensions []string) []*envoy_accesslog_v3.AccessLog {
	// Nil by default to defer to Envoy's default log format.
	var logFormat *envoy_file_v3.FileAccessLog_LogFormat

	if format != "" {
		logFormat = &envoy_file_v3.FileAccessLog_LogFormat{
			LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
				Format: &envoy_config_core_v3.SubstitutionFormatString_TextFormatSource{
					TextFormatSource: &envoy_config_core_v3.DataSource{
						Specifier: &envoy_config_core_v3.DataSource_InlineString{
							InlineString: format,
						},
					},
				},
				Formatters: extensionConfig(extensions),
			},
		}
	}

	return []*envoy_accesslog_v3.AccessLog{{
		Name: wellknown.FileAccessLog,
		ConfigType: &envoy_accesslog_v3.AccessLog_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_file_v3.FileAccessLog{
				Path:            path,
				AccessLogFormat: logFormat,
			}),
		},
	}}
}

// FileAccessLogJSON returns a new file based access log filter
// that will log in JSON format
func FileAccessLogJSON(path string, fields config.AccessLogFields, extensions []string) []*envoy_accesslog_v3.AccessLog {

	jsonformat := &_struct.Struct{
		Fields: make(map[string]*_struct.Value),
	}

	for k, v := range fields.AsFieldMap() {
		jsonformat.Fields[k] = sv(v)
	}

	return []*envoy_accesslog_v3.AccessLog{{
		Name: wellknown.FileAccessLog,
		ConfigType: &envoy_accesslog_v3.AccessLog_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&envoy_file_v3.FileAccessLog{
				Path: path,
				AccessLogFormat: &envoy_file_v3.FileAccessLog_LogFormat{
					LogFormat: &envoy_config_core_v3.SubstitutionFormatString{
						Format: &envoy_config_core_v3.SubstitutionFormatString_JsonFormat{
							JsonFormat: jsonformat,
						},
						Formatters: extensionConfig(extensions),
					},
				},
			}),
		},
	}}
}

func sv(s string) *_struct.Value {
	return &_struct.Value{
		Kind: &_struct.Value_StringValue{
			StringValue: s,
		},
	}
}

// extensionConfig returns a list of extension configs required by the access log format.
//
// Note: When adding support for new formatter, update the list of extensions here and
// add the corresponding extension in pkg/config/parameters.go AccessLogFormatterExtensions().
// Currently only one extension exist in Envoy.
func extensionConfig(extensions []string) []*envoy_config_core_v3.TypedExtensionConfig {
	var config []*envoy_config_core_v3.TypedExtensionConfig

	for _, e := range extensions {
		if e == "envoy.formatter.req_without_query" {
			config = append(config, &envoy_config_core_v3.TypedExtensionConfig{
				Name:        "envoy.formatter.req_without_query",
				TypedConfig: protobuf.MustMarshalAny(&envoy_req_without_query_v3.ReqWithoutQuery{ /* empty */ }),
			})
		}
	}

	return config
}
