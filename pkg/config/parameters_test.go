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

package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestGetenvOr(t *testing.T) {
	assert.Equal(t, t.Name(), GetenvOr("B5E09AAD-DEFC-4650-9DE6-0F2E3AF7FCF2", t.Name()))

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		assert.NotEqual(t, t.Name(), GetenvOr(parts[0], t.Name()))
	}
}

func TestParseDefaults(t *testing.T) {
	savedHome := os.Getenv("HOME")
	defer func() {
		os.Setenv("HOME", savedHome)
	}()

	require.NoError(t, os.Setenv("HOME", t.Name()))

	data, err := yaml.Marshal(Defaults())
	require.NoError(t, err)

	expected := `
debug: false
kubeconfig: TestParseDefaults/.kube/config
server:
  xds-server-type: contour
accesslog-format: envoy
json-fields:
- '@timestamp'
- authority
- bytes_received
- bytes_sent
- downstream_local_address
- downstream_remote_address
- duration
- method
- path
- protocol
- request_id
- requested_server_name
- response_code
- response_flags
- uber_trace_id
- upstream_cluster
- upstream_host
- upstream_local_address
- upstream_service_time
- user_agent
- x_forwarded_for
leaderelection:
  lease-duration: 15s
  renew-deadline: 10s
  retry-period: 2s
  configmap-namespace: projectcontour
  configmap-name: leader-elect
timeouts:
  connection-idle-timeout: 60s
envoy-service-namespace: projectcontour
envoy-service-name: envoy
default-http-versions: []
cluster:
  dns-lookup-family: auto
`
	assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(data)))

	conf, err := Parse(strings.NewReader(expected))
	require.NoError(t, err)
	require.NoError(t, conf.Validate())

	wanted := Defaults()
	assert.Equal(t, &wanted, conf)
}

func TestParseFailure(t *testing.T) {
	badYAML := `
foo: bad

`
	_, err := Parse(strings.NewReader(badYAML))
	require.Error(t, err)
}

func TestValidateClusterDNSFamilyType(t *testing.T) {
	assert.Error(t, ClusterDNSFamilyType("").Validate())
	assert.Error(t, ClusterDNSFamilyType("foo").Validate())

	assert.NoError(t, AutoClusterDNSFamily.Validate())
	assert.NoError(t, IPv4ClusterDNSFamily.Validate())
	assert.NoError(t, IPv6ClusterDNSFamily.Validate())
}

func TestValidateHeadersPolicy(t *testing.T) {
	assert.Error(t, HeadersPolicy{
		Set: map[string]string{
			"inv@lid-header": "ook",
		},
	}.Validate())
	assert.Error(t, HeadersPolicy{
		Remove: []string{"inv@lid-header"},
	}.Validate())
	assert.NoError(t, HeadersPolicy{
		Set:    map[string]string{},
		Remove: []string{},
	}.Validate())
	assert.NoError(t, HeadersPolicy{
		Set: map[string]string{"X-Envoy-Host": "envoy-a12345"},
	}.Validate())
	assert.NoError(t, HeadersPolicy{
		Set: map[string]string{
			"X-Envoy-Host":     "envoy-s12345",
			"l5d-dst-override": "kuard.default.svc.cluster.local:80",
		},
		Remove: []string{"Sensitive-Header"},
	}.Validate())
	assert.NoError(t, HeadersPolicy{
		Set: map[string]string{
			"X-Envoy-Host":     "%HOSTNAME%",
			"l5d-dst-override": "%CONTOUR_SERVICE_NAME%.%CONTOUR_NAMESPACE%.svc.cluster.local:%CONTOUR_SERVICE_PORT%",
		},
	}.Validate())
}

func TestValidateNamespacedName(t *testing.T) {
	assert.NoErrorf(t, NamespacedName{}.Validate(), "empty name should be OK")
	assert.NoError(t, NamespacedName{Name: "name", Namespace: "ns"}.Validate())

	assert.Error(t, NamespacedName{Name: "name"}.Validate())
	assert.Error(t, NamespacedName{Namespace: "ns"}.Validate())
}

func TestValidateServerType(t *testing.T) {
	assert.Error(t, ServerType("").Validate())
	assert.Error(t, ServerType("foo").Validate())

	assert.NoError(t, EnvoyServerType.Validate())
	assert.NoError(t, ContourServerType.Validate())
}

func TestValidateGatewayParameters(t *testing.T) {
	// Namespace and controllerName are required if name is passed.
	gw := &GatewayParameters{Name: "gwname", Namespace: "", ControllerName: ""}
	assert.EqualError(t, gw.Validate(), "invalid Gateway parameters specified: namespace required, controllerName required")

	// Name and controllerName are required if namespace is passed.
	gw = &GatewayParameters{Name: "", Namespace: "ns", ControllerName: ""}
	assert.EqualError(t, gw.Validate(), "invalid Gateway parameters specified: name required, controllerName required")

	// Name and namespace are required if controllerName is passed.
	gw = &GatewayParameters{Name: "", Namespace: "", ControllerName: "controller"}
	assert.EqualError(t, gw.Validate(), "invalid Gateway parameters specified: name required, namespace required")

	// Name is required if controllerName and namespace are passed.
	gw = &GatewayParameters{ControllerName: "controller", Name: "", Namespace: "ns"}
	assert.EqualError(t, gw.Validate(), "invalid Gateway parameters specified: name required")

	// Namespace is required if controllerName and name are passed.
	gw = &GatewayParameters{ControllerName: "controller", Name: "gwname", Namespace: ""}
	assert.EqualError(t, gw.Validate(), "invalid Gateway parameters specified: namespace required")

	// ControllerName is required.
	gw = &GatewayParameters{ControllerName: "controller", Name: "gwname", Namespace: "ns"}
	assert.Equal(t, nil, gw.Validate())

	// Not required if nothing is passed.
	gw = &GatewayParameters{ControllerName: "", Name: "", Namespace: ""}
	assert.Equal(t, nil, gw.Validate())
}

func TestValidateAccessLogType(t *testing.T) {
	assert.Error(t, AccessLogType("").Validate())
	assert.Error(t, AccessLogType("foo").Validate())

	assert.NoError(t, EnvoyAccessLog.Validate())
	assert.NoError(t, JSONAccessLog.Validate())
}

func TestValidateAccessLogFields(t *testing.T) {
	errorCases := [][]string{
		{"dog", "cat"},
		{"req"},
		{"resp"},
		{"trailer"},
		{"@timestamp", "dog"},
		{"@timestamp", "content-id=%REQ=dog%"},
		{"@timestamp", "content-id=%dog(%"},
		{"@timestamp", "content-id=%REQ()%"},
		{"@timestamp", "content-id=%DOG%"},
		{"@timestamp", "duration=my durations % are %DURATION%.0 and %REQ(:METHOD)%"},
		{"invalid=%REQ%"},
		{"invalid=%TRAILER%"},
		{"invalid=%RESP%"},
		{"invalid=%REQ_WITHOUT_QUERY%"},
		{"@timestamp", "invalid=%START_TIME(%s.%6f):10%"},
	}

	for _, c := range errorCases {
		assert.Error(t, AccessLogFields(c).Validate(), c)
	}

	successCases := [][]string{
		{"@timestamp", "method"},
		{"start_time"},
		{"@timestamp", "response_duration"},
		{"@timestamp", "duration=%DURATION%.0"},
		{"@timestamp", "duration=My duration=%DURATION%.0"},
		{"@timestamp", "duration=%START_TIME(%s.%6f)%"},
		{"@timestamp", "content-id=%REQ(X-CONTENT-ID)%"},
		{"@timestamp", "content-id=%REQ(X-CONTENT-ID):10%"},
		{"@timestamp", "length=%RESP(CONTENT-LENGTH):10%"},
		{"@timestamp", "trailer=%TRAILER(CONTENT-LENGTH):10%"},
		{"@timestamp", "duration=my durations are %DURATION%.0 and method is %REQ(:METHOD)%"},
		{"path=%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%"},
		{"dog=pug", "cat=black"},
	}

	for _, c := range successCases {
		assert.NoError(t, AccessLogFields(c).Validate(), c)
	}
}

func TestValidateHTTPVersionType(t *testing.T) {
	assert.Error(t, HTTPVersionType("").Validate())
	assert.Error(t, HTTPVersionType("foo").Validate())
	assert.Error(t, HTTPVersionType("HTTP/1.1").Validate())
	assert.Error(t, HTTPVersionType("HTTP/2").Validate())

	assert.NoError(t, HTTPVersion1.Validate())
	assert.NoError(t, HTTPVersion2.Validate())
}

func TestValidateTimeoutParams(t *testing.T) {
	assert.NoError(t, TimeoutParameters{}.Validate())
	assert.NoError(t, TimeoutParameters{
		RequestTimeout:                "infinite",
		ConnectionIdleTimeout:         "infinite",
		StreamIdleTimeout:             "infinite",
		MaxConnectionDuration:         "infinite",
		DelayedCloseTimeout:           "infinite",
		ConnectionShutdownGracePeriod: "infinite",
	}.Validate())
	assert.NoError(t, TimeoutParameters{
		RequestTimeout:                "infinity",
		ConnectionIdleTimeout:         "infinity",
		StreamIdleTimeout:             "infinity",
		MaxConnectionDuration:         "infinity",
		DelayedCloseTimeout:           "infinity",
		ConnectionShutdownGracePeriod: "infinity",
	}.Validate())

	assert.Error(t, TimeoutParameters{RequestTimeout: "foo"}.Validate())
	assert.Error(t, TimeoutParameters{ConnectionIdleTimeout: "bar"}.Validate())
	assert.Error(t, TimeoutParameters{StreamIdleTimeout: "baz"}.Validate())
	assert.Error(t, TimeoutParameters{MaxConnectionDuration: "boop"}.Validate())
	assert.Error(t, TimeoutParameters{DelayedCloseTimeout: "bebop"}.Validate())
	assert.Error(t, TimeoutParameters{ConnectionShutdownGracePeriod: "bong"}.Validate())

}

func TestTLSParametersValidation(t *testing.T) {
	// Fallback certificate validation
	assert.NoError(t, TLSParameters{
		FallbackCertificate: NamespacedName{
			Name:      "  ",
			Namespace: "  ",
		},
	}.Validate())
	assert.Error(t, TLSParameters{
		FallbackCertificate: NamespacedName{
			Name:      "somename",
			Namespace: "  ",
		},
	}.Validate())

	// Client certificate validation
	assert.NoError(t, TLSParameters{
		ClientCertificate: NamespacedName{
			Name:      "  ",
			Namespace: "  ",
		},
	}.Validate())
	assert.Error(t, TLSParameters{
		ClientCertificate: NamespacedName{
			Name:      "",
			Namespace: "somenamespace  ",
		},
	}.Validate())

	// Cipher suites validation
	assert.NoError(t, TLSParameters{
		CipherSuites: []string{},
	}.Validate())
	assert.NoError(t, TLSParameters{
		CipherSuites: []string{
			"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]",
			"ECDHE-ECDSA-AES128-GCM-SHA256",
			"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
			"ECDHE-RSA-AES128-GCM-SHA256",
			"ECDHE-ECDSA-AES128-SHA",
			" ECDHE-RSA-AES128-SHA   ",
			"AES128-GCM-SHA256",
			"AES128-SHA",
			"ECDHE-ECDSA-AES256-GCM-SHA384",
			"ECDHE-RSA-AES256-GCM-SHA384",
			"ECDHE-ECDSA-AES256-SHA",
			"ECDHE-RSA-AES256-SHA",
			"AES256-GCM-SHA384",
			"AES256-SHA",
		},
	}.Validate())
	assert.Error(t, TLSParameters{
		CipherSuites: []string{
			"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
			"NOTAVALIDCIPHER",
			"AES128-GCM-SHA256",
		},
	}.Validate())
}

func TestSanitizeCipherSuites(t *testing.T) {
	testCases := map[string]struct {
		ciphers []string
		want    []string
	}{
		"no ciphers": {
			ciphers: nil,
			want:    DefaultTLSCiphers,
		},
		"valid list": {
			ciphers: []string{
				"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
				"  ECDHE-RSA-AES128-SHA ",
				"AES128-SHA",
			},
			want: []string{
				"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]",
				"ECDHE-RSA-AES128-SHA",
				"AES128-SHA",
			},
		},
		"cipher duplicated": {
			ciphers: []string{
				"ECDHE-RSA-AES128-SHA",
				"ECDHE-RSA-AES128-SHA",
			},
			want: []string{
				"ECDHE-RSA-AES128-SHA",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, SanitizeCipherSuites(tc.ciphers))
		})
	}
}

func TestConfigFileValidation(t *testing.T) {
	check := func(yamlIn string) {
		t.Helper()

		conf, err := Parse(strings.NewReader(yamlIn))
		require.NoError(t, err)
		require.Error(t, conf.Validate())
	}

	check(`
cluster:
  dns-lookup-family: stone
`)

	check(`
server:
  xds-server-type: magic
`)

	check(`
accesslog-format: /dev/null
`)

	check(`
json-fields:
- one
`)

	check(`
tls:
  fallback-certificate:
    name: foo
`)

	check(`
tls:
  envoy-client-certificate:
    name: foo
`)

	check(`
tls:
  cipher-suites:
  - NOTVALID
`)

	check(`
timeouts:
  request-timeout: none
`)

	check(`
default-http-versions:
- http/0.9
`)

}

func TestConfigFileDefaultOverrideImport(t *testing.T) {
	check := func(verifier func(*testing.T, *Parameters), yamlIn string) {
		t.Helper()

		conf, err := Parse(strings.NewReader(yamlIn))

		require.NoError(t, err)
		verifier(t, conf)
	}

	check(func(t *testing.T, conf *Parameters) {
		wanted := Defaults()
		assert.Equal(t, &wanted, conf)
	}, "")

	check(func(t *testing.T, conf *Parameters) {
		wanted := Defaults()
		assert.Equal(t, &wanted, conf)
	}, `
incluster: false
disablePermitInsecure: false
disableAllowChunkedLength: false
leaderelection:
  configmap-name: leader-elect
  configmap-namespace: projectcontour
  lease-duration: 15s
  renew-deadline: 10s
  retry-period: 2s
`,
	)

	check(func(t *testing.T, conf *Parameters) {
		wanted := Defaults()
		assert.Equal(t, &wanted, conf)
	}, `
tls:
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, "1.3", conf.TLS.MinimumProtocolVersion)
		assert.Equal(t, TLSCiphers{"ECDHE-RSA-AES256-GCM-SHA384"}, conf.TLS.CipherSuites)
	}, `
tls:
  minimum-protocol-version: 1.3
  cipher-suites:
  - ECDHE-RSA-AES256-GCM-SHA384
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, "foo", conf.LeaderElection.Name)
		assert.Equal(t, "bar", conf.LeaderElection.Namespace)
	}, `
leaderelection:
  configmap-name: foo
  configmap-namespace: bar
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, conf.LeaderElection,
			LeaderElectionParameters{
				Name:          "foo",
				Namespace:     "bar",
				LeaseDuration: 600 * time.Second,
				RenewDeadline: 500 * time.Second,
				RetryPeriod:   60 * time.Second,
			})
	}, `
leaderelection:
  configmap-name: foo
  configmap-namespace: bar
  lease-duration: 600s
  renew-deadline: 500s
  retry-period: 60s
`)
	check(func(t *testing.T, conf *Parameters) {
		assert.ElementsMatch(t,
			[]HTTPVersionType{HTTPVersion1, HTTPVersion2, HTTPVersion2, HTTPVersion1},
			conf.DefaultHTTPVersions,
		)
	}, `
default-http-versions:
- http/1.1
- http/2
- HTTP/2
- HTTP/1.1
`)

	check(func(t *testing.T, conf *Parameters) {
		assert.Equal(t, uint32(1), conf.Network.XffNumTrustedHops)
	}, `
network:
  num-trusted-hops: 1
`)
}

func TestAccessLogFormatString(t *testing.T) {
	errorCases := []string{
		"%REQ=dog%\n",
		"%dog(%\n",
		"%REQ()%\n",
		"%DOG%\n",
		"my durations % are %DURATION%.0 and %REQ(:METHOD)%\n",
		"%REQ%\n",
		"%TRAILER%\n",
		"%RESP%\n",
		"%REQ_WITHOUT_QUERY%\n",
		"%START_TIME(%s.%6f):10%\n",
		"no newline at the end",
	}

	for _, c := range errorCases {
		assert.Error(t, validateAccessLogFormatString(c), c)
	}

	successCases := []string{
		"%DURATION%.0\n",
		"My duration %DURATION%.0\n",
		"%START_TIME(%s.%6f)%\n",
		"%REQ(X-CONTENT-ID)%\n",
		"%REQ(X-CONTENT-ID):10%\n",
		"%RESP(CONTENT-LENGTH):10%\n",
		"%TRAILER(CONTENT-LENGTH):10%\n",
		"my durations are %DURATION%.0 and method is %REQ(:METHOD)%\n",
		"queries %REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)% removed\n",
		"just a string\n",
	}

	for _, c := range successCases {
		assert.NoError(t, validateAccessLogFormatString(c), c)
	}
}

// TestAccessLogFormatExtensions tests that command operators requiring extensions are recognized for given access log format.
func TestAccessLogFormatExtensions(t *testing.T) {
	p1 := Parameters{
		AccessLogFormat:       EnvoyAccessLog,
		AccessLogFormatString: "[%START_TIME%] \"%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%\"\n",
	}
	assert.Equal(t, []string{"envoy.formatter.req_without_query"}, p1.AccessLogFormatterExtensions())

	p2 := Parameters{
		AccessLogFormat: JSONAccessLog,
		AccessLogFields: []string{"@timestamp", "path=%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%"},
	}
	assert.Equal(t, []string{"envoy.formatter.req_without_query"}, p2.AccessLogFormatterExtensions())

	p3 := Defaults()
	assert.Empty(t, p3.AccessLogFormatterExtensions())
}
