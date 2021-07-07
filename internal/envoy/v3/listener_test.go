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
	"testing"
	"time"

	envoy_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_compressor_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	envoy_config_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_tcp_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCodecForVersions(t *testing.T) {
	assert.Equal(t, CodecForVersions(HTTPVersionAuto), HTTPVersionAuto)
	assert.Equal(t, CodecForVersions(HTTPVersion1, HTTPVersion2), HTTPVersionAuto)
	assert.Equal(t, CodecForVersions(HTTPVersion1), HTTPVersion1)
	assert.Equal(t, CodecForVersions(HTTPVersion2), HTTPVersion2)
}

func TestProtoNamesForVersions(t *testing.T) {
	assert.Equal(t, ProtoNamesForVersions(), []string{"h2", "http/1.1"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersionAuto), []string{"h2", "http/1.1"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion1), []string{"http/1.1"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion2), []string{"h2"})
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion3), []string(nil))
	assert.Equal(t, ProtoNamesForVersions(HTTPVersion1, HTTPVersion2), []string{"h2", "http/1.1"})
}

func TestListener(t *testing.T) {
	tests := map[string]struct {
		name, address string
		port          int
		lf            []*envoy_listener_v3.ListenerFilter
		f             []*envoy_listener_v3.Filter
		want          *envoy_listener_v3.Listener
	}{
		"insecure listener": {
			name:    "http",
			address: "0.0.0.0",
			port:    9000,
			f: []*envoy_listener_v3.Filter{
				HTTPConnectionManager("http", FileAccessLogEnvoy("/dev/null", "", nil), 0, 0),
			},
			want: &envoy_listener_v3.Listener{
				Name:    "http",
				Address: SocketAddress("0.0.0.0", 9000),
				FilterChains: FilterChains(
					HTTPConnectionManager("http", FileAccessLogEnvoy("/dev/null", "", nil), 0, 0),
				),
				SocketOptions: TCPKeepaliveSocketOptions(),
			},
		},
		"insecure listener w/ proxy": {
			name:    "http-proxy",
			address: "0.0.0.0",
			port:    9000,
			lf: []*envoy_listener_v3.ListenerFilter{
				ProxyProtocol(),
			},
			f: []*envoy_listener_v3.Filter{
				HTTPConnectionManager("http-proxy", FileAccessLogEnvoy("/dev/null", "", nil), 0, 0),
			},
			want: &envoy_listener_v3.Listener{
				Name:    "http-proxy",
				Address: SocketAddress("0.0.0.0", 9000),
				ListenerFilters: ListenerFilters(
					ProxyProtocol(),
				),
				FilterChains: FilterChains(
					HTTPConnectionManager("http-proxy", FileAccessLogEnvoy("/dev/null", "", nil), 0, 0),
				),
				SocketOptions: TCPKeepaliveSocketOptions(),
			},
		},
		"secure listener": {
			name:    "https",
			address: "0.0.0.0",
			port:    9000,
			lf: ListenerFilters(
				TLSInspector(),
			),
			want: &envoy_listener_v3.Listener{
				Name:    "https",
				Address: SocketAddress("0.0.0.0", 9000),
				ListenerFilters: ListenerFilters(
					TLSInspector(),
				),
				SocketOptions: TCPKeepaliveSocketOptions(),
			},
		},
		"secure listener w/ proxy": {
			name:    "https-proxy",
			address: "0.0.0.0",
			port:    9000,
			lf: ListenerFilters(
				ProxyProtocol(),
				TLSInspector(),
			),
			want: &envoy_listener_v3.Listener{
				Name:    "https-proxy",
				Address: SocketAddress("0.0.0.0", 9000),
				ListenerFilters: ListenerFilters(
					ProxyProtocol(),
					TLSInspector(),
				),
				SocketOptions: TCPKeepaliveSocketOptions(),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Listener(tc.name, tc.address, tc.port, tc.lf, tc.f...)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestSocketAddress(t *testing.T) {
	const (
		addr = "foo.example.com"
		port = 8123
	)

	got := SocketAddress(addr, port)
	want := &envoy_core_v3.Address{
		Address: &envoy_core_v3.Address_SocketAddress{
			SocketAddress: &envoy_core_v3.SocketAddress{
				Protocol: envoy_core_v3.SocketAddress_TCP,
				Address:  addr,
				PortSpecifier: &envoy_core_v3.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	require.Equal(t, want, got)

	got = SocketAddress("::", port)
	want = &envoy_core_v3.Address{
		Address: &envoy_core_v3.Address_SocketAddress{
			SocketAddress: &envoy_core_v3.SocketAddress{
				Protocol:   envoy_core_v3.SocketAddress_TCP,
				Address:    "::",
				Ipv4Compat: true, // Set only for ipv6-any "::"
				PortSpecifier: &envoy_core_v3.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	assert.Equal(t, want, got)
}

func TestDownstreamTLSContext(t *testing.T) {
	const subjectName = "client-subject-name"
	ca := []byte("client-ca-cert")

	serverSecret := &dag.Secret{
		Object: &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tls-cert",
				Namespace: "default",
			},
			Data: map[string][]byte{
				v1.TLSCertKey:       []byte("cert"),
				v1.TLSPrivateKeyKey: []byte("key"),
			},
		},
	}

	cipherSuites := []string{
		"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
		"ECDHE-ECDSA-AES256-GCM-SHA384",
	}

	tlsParams := &envoy_tls_v3.TlsParameters{
		TlsMinimumProtocolVersion: envoy_tls_v3.TlsParameters_TLSv1_2,
		TlsMaximumProtocolVersion: envoy_tls_v3.TlsParameters_TLSv1_3,
		CipherSuites:              cipherSuites,
	}

	tlsCertificateSdsSecretConfigs := []*envoy_tls_v3.SdsSecretConfig{{
		Name: envoy.Secretname(serverSecret),
		SdsConfig: &envoy_core_v3.ConfigSource{
			ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
			ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
				ApiConfigSource: &envoy_core_v3.ApiConfigSource{
					ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
					TransportApiVersion: envoy_core_v3.ApiVersion_V3,
					GrpcServices: []*envoy_core_v3.GrpcService{{
						TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
								ClusterName: "contour",
							},
						},
					}},
				},
			},
		},
	}}

	alpnProtocols := []string{"h2", "http/1.1"}
	validationContext := &envoy_tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_tls_v3.CertificateValidationContext{
			TrustedCa: &envoy_core_v3.DataSource{
				Specifier: &envoy_core_v3.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
		},
	}

	peerValidationContext := &dag.PeerValidationContext{
		CACertificate: &dag.Secret{
			Object: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					dag.CACertificateKey: ca,
				},
			},
		},
	}

	// Negative test case: downstream validation should not contain subjectname.
	peerValidationContextWithSubjectName := &dag.PeerValidationContext{
		CACertificate: &dag.Secret{
			Object: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					dag.CACertificateKey: ca,
				},
			},
		},
		SubjectName: subjectName,
	}

	peerValidationContextSkipClientCertValidation := &dag.PeerValidationContext{
		SkipClientCertValidation: true,
	}
	validationContextSkipVerify := &envoy_tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_tls_v3.CertificateValidationContext{
			TrustChainVerification: envoy_tls_v3.CertificateValidationContext_ACCEPT_UNTRUSTED,
		},
	}
	peerValidationContextSkipClientCertValidationWithCA := &dag.PeerValidationContext{
		CACertificate: &dag.Secret{
			Object: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					dag.CACertificateKey: ca,
				},
			},
		},
		SkipClientCertValidation: true,
	}
	validationContextSkipVerifyWithCA := &envoy_tls_v3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoy_tls_v3.CertificateValidationContext{
			TrustChainVerification: envoy_tls_v3.CertificateValidationContext_ACCEPT_UNTRUSTED,
			TrustedCa: &envoy_core_v3.DataSource{
				Specifier: &envoy_core_v3.DataSource_InlineBytes{
					InlineBytes: ca,
				},
			},
		},
	}

	tests := map[string]struct {
		got  *envoy_tls_v3.DownstreamTlsContext
		want *envoy_tls_v3.DownstreamTlsContext
	}{
		"TLS context without client authentication": {
			DownstreamTLSContext(serverSecret, envoy_tls_v3.TlsParameters_TLSv1_2, cipherSuites, nil, "h2", "http/1.1"),
			&envoy_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
				},
			},
		},
		"TLS context with client authentication": {
			DownstreamTLSContext(serverSecret, envoy_tls_v3.TlsParameters_TLSv1_2, cipherSuites, peerValidationContext, "h2", "http/1.1"),
			&envoy_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContext,
				},
				RequireClientCertificate: protobuf.Bool(true),
			},
		},
		"Downstream validation shall not support subjectName validation": {
			DownstreamTLSContext(serverSecret, envoy_tls_v3.TlsParameters_TLSv1_2, cipherSuites, peerValidationContextWithSubjectName, "h2", "http/1.1"),
			&envoy_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContext,
				},
				RequireClientCertificate: protobuf.Bool(true),
			},
		},
		"skip client cert validation": {
			DownstreamTLSContext(serverSecret, envoy_tls_v3.TlsParameters_TLSv1_2, cipherSuites, peerValidationContextSkipClientCertValidation, "h2", "http/1.1"),
			&envoy_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContextSkipVerify,
				},
				RequireClientCertificate: protobuf.Bool(true),
			},
		},
		"skip client cert validation with ca": {
			DownstreamTLSContext(serverSecret, envoy_tls_v3.TlsParameters_TLSv1_2, cipherSuites, peerValidationContextSkipClientCertValidationWithCA, "h2", "http/1.1"),
			&envoy_tls_v3.DownstreamTlsContext{
				CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
					TlsParams:                      tlsParams,
					TlsCertificateSdsSecretConfigs: tlsCertificateSdsSecretConfigs,
					AlpnProtocols:                  alpnProtocols,
					ValidationContextType:          validationContextSkipVerifyWithCA,
				},
				RequireClientCertificate: protobuf.Bool(true),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			protobuf.ExpectEqual(t, tc.want, tc.got)
		})
	}
}

func TestHTTPConnectionManager(t *testing.T) {
	tests := map[string]struct {
		routename                     string
		accesslogger                  []*envoy_accesslog_v3.AccessLog
		requestTimeout                timeout.Setting
		connectionIdleTimeout         timeout.Setting
		streamIdleTimeout             timeout.Setting
		maxConnectionDuration         timeout.Setting
		delayedCloseTimeout           timeout.Setting
		connectionShutdownGracePeriod timeout.Setting
		allowChunkedLength            bool
		xffNumTrustedHops             uint32
		want                          *envoy_listener_v3.Filter
	}{
		"default": {
			routename:    "default/kuard",
			accesslogger: FileAccessLogEnvoy("/dev/stdout", "", nil),
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
					}),
				},
			},
		},
		"request timeout of 10s": {
			routename:      "default/kuard",
			accesslogger:   FileAccessLogEnvoy("/dev/stdout", "", nil),
			requestTimeout: timeout.DurationSetting(10 * time.Second),
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						RequestTimeout:            protobuf.Duration(10 * time.Second),
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
					}),
				},
			},
		},
		"connection idle timeout of 90s": {
			routename:             "default/kuard",
			accesslogger:          FileAccessLogEnvoy("/dev/stdout", "", nil),
			connectionIdleTimeout: timeout.DurationSetting(90 * time.Second),
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{
							IdleTimeout: protobuf.Duration(90 * time.Second),
						},
						AccessLog:        FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress: protobuf.Bool(true),
						NormalizePath:    protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
					}),
				},
			},
		},
		"stream idle timeout of 90s": {
			routename:         "default/kuard",
			accesslogger:      FileAccessLogEnvoy("/dev/stdout", "", nil),
			streamIdleTimeout: timeout.DurationSetting(90 * time.Second),
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						StreamIdleTimeout:         protobuf.Duration(90 * time.Second),
					}),
				},
			},
		},
		"max connection duration of 90s": {
			routename:             "default/kuard",
			accesslogger:          FileAccessLogEnvoy("/dev/stdout", "", nil),
			maxConnectionDuration: timeout.DurationSetting(90 * time.Second),
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{
							MaxConnectionDuration: protobuf.Duration(90 * time.Second),
						},
						AccessLog:        FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress: protobuf.Bool(true),
						NormalizePath:    protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
					}),
				},
			},
		},
		"when max connection duration is disabled, it's omitted": {
			routename:             "default/kuard",
			accesslogger:          FileAccessLogEnvoy("/dev/stdout", "", nil),
			maxConnectionDuration: timeout.DisabledSetting(),
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
					}),
				},
			},
		},
		"delayed close timeout of 90s": {
			routename:           "default/kuard",
			accesslogger:        FileAccessLogEnvoy("/dev/stdout", "", nil),
			delayedCloseTimeout: timeout.DurationSetting(90 * time.Second),
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						DelayedCloseTimeout:       protobuf.Duration(90 * time.Second),
					}),
				},
			},
		},
		"drain timeout of 90s": {
			routename:                     "default/kuard",
			accesslogger:                  FileAccessLogEnvoy("/dev/stdout", "", nil),
			connectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						DrainTimeout:              protobuf.Duration(90 * time.Second),
					}),
				},
			},
		},
		"enable allow_chunked_length": {
			routename:                     "default/kuard",
			accesslogger:                  FileAccessLogEnvoy("/dev/stdout", "", nil),
			connectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
			allowChunkedLength:            true,
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10:      true,
							AllowChunkedLength: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						DrainTimeout:              protobuf.Duration(90 * time.Second),
					}),
				},
			},
		},
		"enable XffNumTrustedHops": {
			routename:                     "default/kuard",
			accesslogger:                  FileAccessLogEnvoy("/dev/stdout", "", nil),
			connectionShutdownGracePeriod: timeout.DurationSetting(90 * time.Second),
			xffNumTrustedHops:             1,
			want: &envoy_listener_v3.Filter{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&http.HttpConnectionManager{
						StatPrefix: "default/kuard",
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: "default/kuard",
								ConfigSource: &envoy_core_v3.ConfigSource{
									ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
									ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_ApiConfigSource{
										ApiConfigSource: &envoy_core_v3.ApiConfigSource{
											ApiType:             envoy_core_v3.ApiConfigSource_GRPC,
											TransportApiVersion: envoy_core_v3.ApiVersion_V3,
											GrpcServices: []*envoy_core_v3.GrpcService{{
												TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
													EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
														ClusterName: "contour",
													},
												},
											}},
										},
									},
								},
							},
						},
						HttpFilters: []*http.HttpFilter{{
							Name: "compressor",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
									CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
										Name: "gzip",
										TypedConfig: &any.Any{
											TypeUrl: HTTPFilterGzip,
										},
									},
								}),
							},
						}, {
							Name: "grpcweb",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGrpcWeb,
								},
							},
						}, {
							Name: "cors",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterCORS,
								},
							},
						}, {
							Name: "local_ratelimit",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: protobuf.MustMarshalAny(
									&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
										StatPrefix: "http",
									},
								),
							},
						}, {
							Name: "router",
							ConfigType: &http.HttpFilter_TypedConfig{
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterRouter,
								},
							},
						}},
						HttpProtocolOptions: &envoy_core_v3.Http1ProtocolOptions{
							// Enable support for HTTP/1.0 requests that carry
							// a Host: header. See #537.
							AcceptHttp_10: true,
						},
						CommonHttpProtocolOptions: &envoy_core_v3.HttpProtocolOptions{},
						AccessLog:                 FileAccessLogEnvoy("/dev/stdout", "", nil),
						UseRemoteAddress:          protobuf.Bool(true),
						NormalizePath:             protobuf.Bool(true),
						StripPortMode: &http.HttpConnectionManager_StripAnyHostPort{
							StripAnyHostPort: true,
						},
						PreserveExternalRequestId: true,
						MergeSlashes:              true,
						DrainTimeout:              protobuf.Duration(90 * time.Second),
						XffNumTrustedHops:         1,
					}),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := HTTPConnectionManagerBuilder().
				RouteConfigName(tc.routename).
				MetricsPrefix(tc.routename).
				AccessLoggers(tc.accesslogger).
				RequestTimeout(tc.requestTimeout).
				ConnectionIdleTimeout(tc.connectionIdleTimeout).
				StreamIdleTimeout(tc.streamIdleTimeout).
				MaxConnectionDuration(tc.maxConnectionDuration).
				DelayedCloseTimeout(tc.delayedCloseTimeout).
				ConnectionShutdownGracePeriod(tc.connectionShutdownGracePeriod).
				AllowChunkedLength(tc.allowChunkedLength).
				NumTrustedHops(tc.xffNumTrustedHops).
				DefaultFilters().
				Get()

			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestTCPProxy(t *testing.T) {
	const (
		statPrefix    = "ingress_https"
		accessLogPath = "/dev/stdout"
	)

	c1 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				Weight:           1,
				ServiceName:      "example",
				ServiceNamespace: "default",
				ServicePort: v1.ServicePort{
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			},
		},
	}
	c2 := &dag.Cluster{
		Upstream: &dag.Service{
			Weighted: dag.WeightedService{
				ServiceName:      "example2",
				ServiceNamespace: "default",
				ServicePort: v1.ServicePort{
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			},
		},
		Weight: 20,
	}

	tests := map[string]struct {
		proxy *dag.TCPProxy
		want  *envoy_listener_v3.Filter
	}{
		"single cluster": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c1},
			},
			want: &envoy_listener_v3.Filter{
				Name: wellknown.TCPProxy,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_tcp_proxy_v3.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_tcp_proxy_v3.TcpProxy_Cluster{
							Cluster: envoy.Clustername(c1),
						},
						AccessLog:   FileAccessLogEnvoy(accessLogPath, "", nil),
						IdleTimeout: protobuf.Duration(9001 * time.Second),
					}),
				},
			},
		},
		"multiple cluster": {
			proxy: &dag.TCPProxy{
				Clusters: []*dag.Cluster{c2, c1},
			},
			want: &envoy_listener_v3.Filter{
				Name: wellknown.TCPProxy,
				ConfigType: &envoy_listener_v3.Filter_TypedConfig{
					TypedConfig: protobuf.MustMarshalAny(&envoy_tcp_proxy_v3.TcpProxy{
						StatPrefix: statPrefix,
						ClusterSpecifier: &envoy_tcp_proxy_v3.TcpProxy_WeightedClusters{
							WeightedClusters: &envoy_tcp_proxy_v3.TcpProxy_WeightedCluster{
								Clusters: []*envoy_tcp_proxy_v3.TcpProxy_WeightedCluster_ClusterWeight{{
									Name:   envoy.Clustername(c1),
									Weight: 1,
								}, {
									Name:   envoy.Clustername(c2),
									Weight: 20,
								}},
							},
						},
						AccessLog:   FileAccessLogEnvoy(accessLogPath, "", nil),
						IdleTimeout: protobuf.Duration(9001 * time.Second),
					}),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := TCPProxy(statPrefix, tc.proxy, FileAccessLogEnvoy(accessLogPath, "", nil))
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

func TestFilterChainTLS_Match(t *testing.T) {

	tests := map[string]struct {
		domain     string
		downstream *envoy_tls_v3.DownstreamTlsContext
		filters    []*envoy_listener_v3.Filter
		want       *envoy_listener_v3.FilterChain
	}{
		"SNI": {
			domain: "projectcontour.io",
			want: &envoy_listener_v3.FilterChain{
				FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
					ServerNames: []string{"projectcontour.io"},
				},
			},
		},
		"No SNI": {
			domain: "*",
			want: &envoy_listener_v3.FilterChain{
				FilterChainMatch: &envoy_listener_v3.FilterChainMatch{
					TransportProtocol: "tls",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FilterChainTLS(tc.domain, tc.downstream, tc.filters)
			protobuf.ExpectEqual(t, tc.want, got)
		})
	}
}

// TestBuilderValidation tests that validation checks that
// DefaultFilters adds the required HTTP connection manager filters.
func TestBuilderValidation(t *testing.T) {

	assert.Error(t, HTTPConnectionManagerBuilder().Validate(),
		"ConnectionManager with no filters should not pass validation")

	assert.Error(t, HTTPConnectionManagerBuilder().AddFilter(&http.HttpFilter{
		Name: "foo",
		ConfigType: &http.HttpFilter_TypedConfig{
			TypedConfig: &any.Any{
				TypeUrl: "foo",
			},
		},
	}).Validate(),
		"ConnectionManager with only non-router filter should not pass validation")

	assert.NoError(t, HTTPConnectionManagerBuilder().DefaultFilters().Validate(),
		"ConnectionManager with default filters failed validation")

	badBuilder := HTTPConnectionManagerBuilder().DefaultFilters()
	badBuilder.filters = append(badBuilder.filters, &http.HttpFilter{
		Name: "foo",
		ConfigType: &http.HttpFilter_TypedConfig{
			TypedConfig: &any.Any{
				TypeUrl: "foo",
			},
		},
	})
	assert.Errorf(t, badBuilder.Validate(), "Adding a filter after the Router filter should fail")
}

func TestAddFilter(t *testing.T) {

	tests := map[string]struct {
		builder *httpConnectionManagerBuilder
		add     *http.HttpFilter
		want    []*http.HttpFilter
	}{
		"Nil add to empty builder": {
			builder: HTTPConnectionManagerBuilder(),
			add:     nil,
			want:    nil,
		},
		"Add a single router filter to empty builder": {
			builder: HTTPConnectionManagerBuilder(),
			add: &http.HttpFilter{
				Name: "router",
				ConfigType: &http.HttpFilter_TypedConfig{
					TypedConfig: &any.Any{
						TypeUrl: HTTPFilterRouter,
					},
				},
			},
			want: []*http.HttpFilter{
				{
					Name: "router",
					ConfigType: &http.HttpFilter_TypedConfig{
						TypedConfig: &any.Any{
							TypeUrl: HTTPFilterRouter,
						},
					},
				},
			},
		},
		"Add a single non-router filter to empty builder": {
			builder: HTTPConnectionManagerBuilder(),
			add: &http.HttpFilter{
				Name: "grpcweb",
				ConfigType: &http.HttpFilter_TypedConfig{
					TypedConfig: &any.Any{
						TypeUrl: HTTPFilterGrpcWeb,
					},
				},
			},
			want: []*http.HttpFilter{
				{
					Name: "grpcweb",
					ConfigType: &http.HttpFilter_TypedConfig{
						TypedConfig: &any.Any{
							TypeUrl: HTTPFilterGrpcWeb,
						},
					},
				},
			},
		},
		"Add a filter to a builder with a router": {
			builder: HTTPConnectionManagerBuilder().AddFilter(&http.HttpFilter{
				Name: "router",
				ConfigType: &http.HttpFilter_TypedConfig{
					TypedConfig: &any.Any{
						TypeUrl: HTTPFilterRouter,
					},
				},
			}),
			add: &http.HttpFilter{
				Name: "grpcweb",
				ConfigType: &http.HttpFilter_TypedConfig{
					TypedConfig: &any.Any{
						TypeUrl: HTTPFilterGrpcWeb,
					},
				},
			},
			want: []*http.HttpFilter{
				{
					Name: "grpcweb",
					ConfigType: &http.HttpFilter_TypedConfig{
						TypedConfig: &any.Any{
							TypeUrl: HTTPFilterGrpcWeb,
						},
					},
				},
				{
					Name: "router",
					ConfigType: &http.HttpFilter_TypedConfig{
						TypedConfig: &any.Any{
							TypeUrl: HTTPFilterRouter,
						},
					},
				},
			},
		},
		"Add to the default filters": {
			builder: HTTPConnectionManagerBuilder().DefaultFilters(),
			add:     FilterExternalAuthz("test", false, timeout.Setting{}),
			want: []*http.HttpFilter{
				{
					Name: "compressor",
					ConfigType: &http.HttpFilter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(&envoy_compressor_v3.Compressor{
							CompressorLibrary: &envoy_core_v3.TypedExtensionConfig{
								Name: "gzip",
								TypedConfig: &any.Any{
									TypeUrl: HTTPFilterGzip,
								},
							},
						}),
					},
				},
				{
					Name: "grpcweb",
					ConfigType: &http.HttpFilter_TypedConfig{
						TypedConfig: &any.Any{
							TypeUrl: HTTPFilterGrpcWeb,
						},
					},
				},
				{
					Name: "cors",
					ConfigType: &http.HttpFilter_TypedConfig{
						TypedConfig: &any.Any{
							TypeUrl: HTTPFilterCORS,
						},
					},
				},
				{
					Name: "local_ratelimit",
					ConfigType: &http.HttpFilter_TypedConfig{
						TypedConfig: protobuf.MustMarshalAny(
							&envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
								StatPrefix: "http",
							},
						),
					},
				},
				FilterExternalAuthz("test", false, timeout.Setting{}),
				{
					Name: "router",
					ConfigType: &http.HttpFilter_TypedConfig{
						TypedConfig: &any.Any{
							TypeUrl: HTTPFilterRouter,
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.builder.AddFilter(tc.add)
			assert.Equal(t, tc.want, got.filters)
		})
	}

	assert.Panics(t, func() {
		HTTPConnectionManagerBuilder().DefaultFilters().AddFilter(&http.HttpFilter{
			Name: "router",
			ConfigType: &http.HttpFilter_TypedConfig{
				TypedConfig: &any.Any{
					TypeUrl: HTTPFilterRouter,
				},
			},
		})
	})
}
