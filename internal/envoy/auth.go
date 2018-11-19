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

package envoy

import (
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
)

// UpstreamTLSContext creates an ALPN h2 enabled TLS Context.
func UpstreamTLSContext() *auth.UpstreamTlsContext {
	return &auth.UpstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			AlpnProtocols: []string{"h2"},
		},
	}
}

// UpstreamTLSContext creates an ALPN h2 enabled TLS Context with TLS verification enabled.
func UpstreamTLSContextWithVerification(cert []byte) *auth.UpstreamTlsContext {
	return &auth.UpstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			ValidationContextType: &auth.CommonTlsContext_ValidationContext{
				ValidationContext: &auth.CertificateValidationContext{
					TrustedCa: &core.DataSource{
						Specifier: &core.DataSource_InlineBytes{
							InlineBytes: cert,
						},
					},
				},
			},
			AlpnProtocols: []string{"h2"},
		},
	}
}
