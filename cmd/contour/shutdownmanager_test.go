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

package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/projectcontour/contour/internal/fixture"
)

func TestShutdownManager_HealthzHandler(t *testing.T) {
	// Create a request to pass to our handler
	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	mgr := shutdownmanagerContext{}

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(mgr.healthzHandler)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
	expected := `OK`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestShutdownManager_NewShutDownContextCheckDefaultAdminPort(t *testing.T) {
	shutdownContext := newShutdownContext()
	if (*shutdownContext).adminPort != 9001 {
		t.Errorf("TestShutdownManager_NewShutDownContextWithoutAdminPort failed: expected shutdown port %d, got %d", 9001, (*shutdownContext).adminPort)
	}
}

func TestShutdownManager_ShutdownReadyHandler_Success(t *testing.T) {
	// Create a request to pass to our handler
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "/shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}

	mgr := newShutdownManagerContext()
	mgr.FieldLogger = fixture.NewTestLogger(t)
	tmpdir, err := ioutil.TempDir("", "shutdownmanager_test-*")
	defer os.RemoveAll(tmpdir)
	if err != nil {
		t.Error(err)
	}
	mgr.shutdownReadyFile = path.Join(tmpdir, "ok")
	mgr.shutdownReadyCheckInterval = time.Millisecond * 20

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(mgr.shutdownReadyHandler)

	go func() {
		time.Sleep(50 * time.Millisecond)
		file, err := os.Create(mgr.shutdownReadyFile)
		if err != nil {
			t.Error(err)
		}
		defer file.Close()
	}()

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
	expected := `OK`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestShutdownManager_ShutdownReadyHandler_ClientCancel(t *testing.T) {
	// Create a request to pass to our handler
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "/shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}

	mgr := newShutdownManagerContext()
	mgr.FieldLogger = fixture.NewTestLogger(t)

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(mgr.shutdownReadyHandler)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)
}

func TestParseOpenConnections(t *testing.T) {
	type testcase struct {
		stats           io.Reader
		wantConnections int
		wantError       error
	}

	run := func(t *testing.T, name string, tc testcase) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Helper()

			gotConnections, gotError := parseOpenConnections(tc.stats)
			assert.Equal(t, tc.wantError, gotError)
			assert.Equal(t, tc.wantConnections, gotConnections)
		})
	}

	run(t, "nil stats", testcase{
		stats:           nil,
		wantConnections: -1,
		wantError:       fmt.Errorf("stats input was nil"),
	})

	run(t, "basic http only", testcase{
		stats:           strings.NewReader(VALIDHTTP),
		wantConnections: 4,
		wantError:       nil,
	})

	run(t, "basic https only", testcase{
		stats:           strings.NewReader(VALIDHTTPS),
		wantConnections: 4,
		wantError:       nil,
	})

	run(t, "basic both protocols", testcase{
		stats:           strings.NewReader(VALIDBOTH),
		wantConnections: 8,
		wantError:       nil,
	})

	run(t, "missing values", testcase{
		stats:           strings.NewReader(MISSING_STATS),
		wantConnections: -1,
		wantError:       fmt.Errorf("error finding Prometheus stat \"envoy_http_downstream_cx_active\" in the request result"),
	})

	run(t, "invalid stats", testcase{
		stats:           strings.NewReader("!!##$$##!!"),
		wantConnections: -1,
		wantError:       fmt.Errorf("parsing Prometheus text format failed: text format parsing error in line 1: invalid metric name"),
	})
}

// nolint:revive
const (
	VALIDHTTP = `envoy_cluster_circuit_breakers_default_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_max_host_weight{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_rq_pending_active{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_rq_retry_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_cx_tx_bytes_buffered{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_version{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_default_cx_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
# TYPE envoy_http_downstream_cx_ssl_active gauge
envoy_http_downstream_cx_ssl_active{envoy_http_conn_manager_prefix="admin"} 0
# TYPE envoy_server_total_connections gauge
envoy_server_total_connections{} 1
# TYPE envoy_runtime_num_layers gauge
envoy_runtime_num_layers{} 2
# TYPE envoy_server_parent_connections gauge
envoy_server_parent_connections{} 0
# TYPE envoy_server_stats_recent_lookups gauge
envoy_server_stats_recent_lookups{} 0
# TYPE envoy_cluster_manager_warming_clusters gauge
envoy_cluster_manager_warming_clusters{} 0
# TYPE envoy_server_days_until_first_cert_expiring gauge
envoy_server_days_until_first_cert_expiring{} 82
# TYPE envoy_server_hot_restart_epoch gauge
envoy_server_hot_restart_epoch{} 0
# TYPE envoy_http_downstream_cx_active gauge
envoy_http_downstream_cx_active{envoy_http_conn_manager_prefix="ingress_http"} 4
`
	VALIDHTTPS = `envoy_cluster_circuit_breakers_default_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_max_host_weight{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_rq_pending_active{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_rq_retry_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_cx_tx_bytes_buffered{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_version{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_default_cx_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
# TYPE envoy_http_downstream_cx_ssl_active gauge
envoy_http_downstream_cx_ssl_active{envoy_http_conn_manager_prefix="admin"} 0
# TYPE envoy_server_total_connections gauge
envoy_server_total_connections{} 1
# TYPE envoy_runtime_num_layers gauge
envoy_runtime_num_layers{} 2
# TYPE envoy_server_parent_connections gauge
envoy_server_parent_connections{} 0
# TYPE envoy_server_stats_recent_lookups gauge
envoy_server_stats_recent_lookups{} 0
# TYPE envoy_cluster_manager_warming_clusters gauge
envoy_cluster_manager_warming_clusters{} 0
# TYPE envoy_server_days_until_first_cert_expiring gauge
envoy_server_days_until_first_cert_expiring{} 82
# TYPE envoy_server_hot_restart_epoch gauge
envoy_server_hot_restart_epoch{} 0
# TYPE envoy_http_downstream_cx_active gauge
envoy_http_downstream_cx_active{envoy_http_conn_manager_prefix="ingress_http"} 4
`
	VALIDBOTH = `envoy_cluster_circuit_breakers_default_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_max_host_weight{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_rq_pending_active{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_rq_retry_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_cx_tx_bytes_buffered{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_version{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_default_cx_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
# TYPE envoy_http_downstream_cx_ssl_active gauge
envoy_http_downstream_cx_ssl_active{envoy_http_conn_manager_prefix="admin"} 0
# TYPE envoy_server_total_connections gauge
envoy_server_total_connections{} 1
# TYPE envoy_runtime_num_layers gauge
envoy_runtime_num_layers{} 2
# TYPE envoy_server_parent_connections gauge
envoy_server_parent_connections{} 0
# TYPE envoy_server_stats_recent_lookups gauge
envoy_server_stats_recent_lookups{} 0
# TYPE envoy_cluster_manager_warming_clusters gauge
envoy_cluster_manager_warming_clusters{} 0
# TYPE envoy_server_days_until_first_cert_expiring gauge
envoy_server_days_until_first_cert_expiring{} 82
# TYPE envoy_server_hot_restart_epoch gauge
envoy_server_hot_restart_epoch{} 0
# TYPE envoy_http_downstream_cx_active gauge
envoy_http_downstream_cx_active{envoy_http_conn_manager_prefix="ingress_http"} 4
envoy_http_downstream_cx_active{envoy_http_conn_manager_prefix="ingress_https"} 4
`

	MISSING_STATS = `envoy_cluster_circuit_breakers_default_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_max_host_weight{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_rq_pending_active{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_rq_retry_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_high_cx_pool_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_upstream_cx_tx_bytes_buffered{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_version{envoy_cluster_name="projectcontour_service-stats_9001"} 0
envoy_cluster_circuit_breakers_default_cx_open{envoy_cluster_name="projectcontour_service-stats_9001"} 0
# TYPE envoy_http_downstream_cx_ssl_active gauge
envoy_http_downstream_cx_ssl_active{envoy_http_conn_manager_prefix="admin"} 0
# TYPE envoy_server_total_connections gauge
envoy_server_total_connections{} 1
# TYPE envoy_runtime_num_layers gauge
envoy_runtime_num_layers{} 2
# TYPE envoy_server_parent_connections gauge
envoy_server_parent_connections{} 0
# TYPE envoy_server_stats_recent_lookups gauge
envoy_server_stats_recent_lookups{} 0
# TYPE envoy_cluster_manager_warming_clusters gauge
envoy_cluster_manager_warming_clusters{} 0
# TYPE envoy_server_days_until_first_cert_expiring gauge
envoy_server_days_until_first_cert_expiring{} 82
# TYPE envoy_server_hot_restart_epoch gauge
envoy_server_hot_restart_epoch{} 0
`
)
