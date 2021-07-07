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

// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega/gexec"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contourv1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensions_v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	// needed if tests are run against GCP
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

// Framework provides a collection of helpful functions for
// writing end-to-end (E2E) tests for Contour.
type Framework struct {
	// Client is a controller-runtime Kubernetes client.
	Client client.Client

	// RetryInterval is how often to retry polling operations.
	RetryInterval time.Duration

	// RetryTimeout is how long to continue trying polling
	// operations before giving up.
	RetryTimeout time.Duration

	// Fixtures provides helpers for working with test fixtures,
	// i.e. sample workloads that can be used as proxy targets.
	Fixtures *Fixtures

	// HTTP provides helpers for making HTTP/HTTPS requests.
	HTTP *HTTP

	// Certs provides helpers for creating cert-manager certificates
	// and related resources.
	Certs *Certs

	// Deployment provides helpers for managing deploying resources that
	// are part of a full Contour deployment manifest.
	Deployment *Deployment

	t ginkgo.GinkgoTInterface
}

func NewFramework(inClusterTestSuite bool) *Framework {
	t := ginkgo.GinkgoT()

	scheme := runtime.NewScheme()
	require.NoError(t, kubescheme.AddToScheme(scheme))
	require.NoError(t, contourv1.AddToScheme(scheme))
	require.NoError(t, contourv1alpha1.AddToScheme(scheme))
	require.NoError(t, gatewayv1alpha1.AddToScheme(scheme))
	require.NoError(t, certmanagerv1.AddToScheme(scheme))
	require.NoError(t, apiextensions_v1.AddToScheme(scheme))

	config, err := config.GetConfig()
	require.NoError(t, err)

	configQPS := os.Getenv("K8S_CLIENT_QPS")
	if configQPS == "" {
		configQPS = "100"
	}

	configBurst := os.Getenv("K8S_CLIENT_BURST")
	if configBurst == "" {
		configBurst = "100"
	}

	qps, err := strconv.ParseFloat(configQPS, 32)
	require.NoError(t, err)

	burst, err := strconv.Atoi(configBurst)
	require.NoError(t, err)

	config.QPS = float32(qps)
	config.Burst = burst

	crClient, err := client.New(config, client.Options{Scheme: scheme})
	require.NoError(t, err)

	httpURLBase := os.Getenv("CONTOUR_E2E_HTTP_URL_BASE")
	if httpURLBase == "" {
		httpURLBase = "http://127.0.0.1:9080"
	}

	httpsURLBase := os.Getenv("CONTOUR_E2E_HTTPS_URL_BASE")
	if httpsURLBase == "" {
		httpsURLBase = "https://127.0.0.1:9443"
	}

	var (
		kubeConfig   string
		contourHost  string
		contourPort  string
		contourBin   string
		contourImage string
	)
	if inClusterTestSuite {
		var found bool
		if contourImage, found = os.LookupEnv("CONTOUR_E2E_IMAGE"); !found {
			contourImage = "projectcontour/contour:main"
		}
	} else {
		var found bool
		if kubeConfig, found = os.LookupEnv("KUBECONFIG"); !found {
			kubeConfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}

		contourHost = os.Getenv("CONTOUR_E2E_LOCAL_HOST")
		require.NotEmpty(t, contourHost, "CONTOUR_E2E_LOCAL_HOST environment variable not supplied")

		if contourPort, found = os.LookupEnv("CONTOUR_E2E_LOCAL_PORT"); !found {
			contourPort = "8001"
		}

		var err error
		contourBin, err = gexec.Build("github.com/projectcontour/contour/cmd/contour")
		require.NoError(t, err)
	}

	deployment := &Deployment{
		client:           crClient,
		logOutputWriter:  ginkgo.GinkgoWriter,
		kubeConfig:       kubeConfig,
		localContourHost: contourHost,
		localContourPort: contourPort,
		contourBin:       contourBin,
		contourImage:     contourImage,
	}
	require.NoError(t, deployment.UnmarshalResources())

	return &Framework{
		Client:        crClient,
		RetryInterval: time.Second,
		RetryTimeout:  60 * time.Second,
		Fixtures: &Fixtures{
			Echo: &Echo{
				client: crClient,
				t:      t,
			},
			EchoSecure: &EchoSecure{
				client: crClient,
				t:      t,
			},
			HTTPBin: &HTTPBin{
				client: crClient,
				t:      t,
			},
		},
		HTTP: &HTTP{
			HTTPURLBase:   httpURLBase,
			HTTPSURLBase:  httpsURLBase,
			RetryInterval: time.Second,
			RetryTimeout:  60 * time.Second,
			t:             t,
		},
		Certs: &Certs{
			client:        crClient,
			retryInterval: time.Second,
			retryTimeout:  60 * time.Second,
			t:             t,
		},
		Deployment: deployment,
		t:          t,
	}
}

// T exposes a GinkgoTInterface which exposes many of the same methods
// as a *testing.T, for use in tests that previously required a *testing.T.
func (f *Framework) T() ginkgo.GinkgoTInterface {
	return f.t
}

type NamespacedTestBody func(string)

func (f *Framework) NamespacedTest(namespace string, body NamespacedTestBody) {
	ginkgo.Context("with namespace: "+namespace, func() {
		ginkgo.BeforeEach(func() {
			f.CreateNamespace(namespace)
		})
		ginkgo.AfterEach(func() {
			f.DeleteNamespace(namespace, false)
		})

		body(namespace)
	})
}

// CreateHTTPProxyAndWaitFor creates the provided HTTPProxy in the Kubernetes API
// and then waits for the specified condition to be true.
func (f *Framework) CreateHTTPProxyAndWaitFor(proxy *contourv1.HTTPProxy, condition func(*contourv1.HTTPProxy) bool) (*contourv1.HTTPProxy, bool) {
	require.NoError(f.t, f.Client.Create(context.TODO(), proxy))

	res := &contourv1.HTTPProxy{}

	if err := wait.PollImmediate(f.RetryInterval, f.RetryTimeout, func() (bool, error) {
		if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(proxy), res); err != nil {
			// if there was an error, we want to keep
			// retrying, so just return false, not an
			// error.
			return false, nil
		}

		return condition(res), nil
	}); err != nil {
		// return the last response for logging/debugging purposes
		return res, false
	}

	return res, true
}

// CreateHTTPRouteAndWaitFor creates the provided HTTPRoute in the Kubernetes API
// and then waits for the specified condition to be true.
func (f *Framework) CreateHTTPRouteAndWaitFor(route *gatewayv1alpha1.HTTPRoute, condition func(*gatewayv1alpha1.HTTPRoute) bool) (*gatewayv1alpha1.HTTPRoute, bool) {
	require.NoError(f.t, f.Client.Create(context.TODO(), route))

	res := &gatewayv1alpha1.HTTPRoute{}

	if err := wait.PollImmediate(f.RetryInterval, f.RetryTimeout, func() (bool, error) {
		if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(route), res); err != nil {
			// if there was an error, we want to keep
			// retrying, so just return false, not an
			// error.
			return false, nil
		}

		return condition(res), nil
	}); err != nil {
		// return the last response for logging/debugging purposes
		return res, false
	}

	return res, true
}

// CreateTLSRouteAndWaitFor creates the provided TLSRoute in the Kubernetes API
// and then waits for the specified condition to be true.
func (f *Framework) CreateTLSRouteAndWaitFor(route *gatewayv1alpha1.TLSRoute, condition func(*gatewayv1alpha1.TLSRoute) bool) (*gatewayv1alpha1.TLSRoute, bool) {
	require.NoError(f.t, f.Client.Create(context.TODO(), route))

	res := &gatewayv1alpha1.TLSRoute{}

	if err := wait.PollImmediate(f.RetryInterval, f.RetryTimeout, func() (bool, error) {
		if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(route), res); err != nil {
			// if there was an error, we want to keep
			// retrying, so just return false, not an
			// error.
			return false, nil
		}

		return condition(res), nil
	}); err != nil {
		// return the last response for logging/debugging purposes
		return res, false
	}

	return res, true
}

// Helper functions for various common conditions.

// HTTPProxyValid returns true if the proxy has a .status.currentStatus
// of "valid".
func HTTPProxyValid(proxy *contourv1.HTTPProxy) bool {
	return proxy != nil && proxy.Status.CurrentStatus == "valid"
}

// HTTPProxyInvalid returns true if the proxy has a .status.currentStatus
// of "valid".
func HTTPProxyInvalid(proxy *contourv1.HTTPProxy) bool {
	return proxy != nil && proxy.Status.CurrentStatus == "invalid"
}

// DetailedConditionInvalid returns true if the provided detailed condition
// list contains a condition of type "Valid" and status "False".
func DetailedConditionInvalid(conditions []contourv1.DetailedCondition) bool {
	for _, c := range conditions {
		if c.Condition.Type == "Valid" {
			return c.Condition.Status == "False"
		}
	}
	return false
}

// CreateNamespace creates a namespace with the given name in the
// Kubernetes API or fails the test if it encounters an error.
func (f *Framework) CreateNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"contour-e2e-ns": "true"},
		},
	}
	require.NoError(f.t, f.Client.Create(context.TODO(), ns))
}

// DeleteNamespace deletes the namespace with the given name in the
// Kubernetes API or fails the test if it encounters an error.
func (f *Framework) DeleteNamespace(name string, waitForDeletion bool) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	require.NoError(f.t, f.Client.Delete(context.TODO(), ns))

	if waitForDeletion {
		require.Eventually(f.t, func() bool {
			err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(ns), ns)
			return api_errors.IsNotFound(err)
		}, time.Minute*3, time.Millisecond*50)
	}
}

// CreateGatewayAndWaitFor creates a gateway in the
// Kubernetes API or fails the test if it encounters an error.
func (f *Framework) CreateGatewayAndWaitFor(gateway *gatewayv1alpha1.Gateway, condition func(*gatewayv1alpha1.Gateway) bool) (*gatewayv1alpha1.Gateway, bool) {
	require.NoError(f.t, f.Client.Create(context.TODO(), gateway))

	res := &gatewayv1alpha1.Gateway{}

	if err := wait.PollImmediate(f.RetryInterval, f.RetryTimeout, func() (bool, error) {
		if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(gateway), res); err != nil {
			// if there was an error, we want to keep
			// retrying, so just return false, not an
			// error.
			return false, nil
		}

		return condition(res), nil
	}); err != nil {
		// return the last response for logging/debugging purposes
		return res, false
	}

	return res, true
}

// CreateGatewayClassAndWaitFor creates a GatewayClass in the
// Kubernetes API or fails the test if it encounters an error.
func (f *Framework) CreateGatewayClassAndWaitFor(gatewayClass *gatewayv1alpha1.GatewayClass, condition func(*gatewayv1alpha1.GatewayClass) bool) (*gatewayv1alpha1.GatewayClass, bool) {
	require.NoError(f.t, f.Client.Create(context.TODO(), gatewayClass))

	res := &gatewayv1alpha1.GatewayClass{}

	if err := wait.PollImmediate(f.RetryInterval, f.RetryTimeout, func() (bool, error) {
		if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(gatewayClass), res); err != nil {
			// if there was an error, we want to keep
			// retrying, so just return false, not an
			// error.
			return false, nil
		}

		return condition(res), nil
	}); err != nil {
		// return the last response for logging/debugging purposes
		return res, false
	}

	return res, true
}

// DeleteGateway deletes the provided gateway in the Kubernetes API
// or fails the test if it encounters an error.
func (f *Framework) DeleteGateway(gw *gatewayv1alpha1.Gateway, waitForDeletion bool) error {
	require.NoError(f.t, f.Client.Delete(context.TODO(), gw))

	if waitForDeletion {
		require.Eventually(f.t, func() bool {
			err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(gw), gw)
			return api_errors.IsNotFound(err)
		}, time.Minute*3, time.Millisecond*50)
	}
	return nil
}

// DeleteGatewayClass deletes the provided gatewayclass in the
// Kubernetes API or fails the test if it encounters an error.
func (f *Framework) DeleteGatewayClass(gwc *gatewayv1alpha1.GatewayClass, waitForDeletion bool) error {
	require.NoError(f.t, f.Client.Delete(context.TODO(), gwc))

	if waitForDeletion {
		require.Eventually(f.t, func() bool {
			err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(gwc), gwc)
			return api_errors.IsNotFound(err)
		}, time.Minute*3, time.Millisecond*50)
	}

	return nil
}

// GetEchoResponseBody decodes an HTTP response body that is
// expected to have come from ingress-conformance-echo into an
// EchoResponseBody, or fails the test if it encounters an error.
func (f *Framework) GetEchoResponseBody(body []byte) EchoResponseBody {
	var echoBody EchoResponseBody

	require.NoError(f.t, json.Unmarshal(body, &echoBody))

	return echoBody
}

type EchoResponseBody struct {
	Path           string      `json:"path"`
	Host           string      `json:"host"`
	RequestHeaders http.Header `json:"headers"`
	Namespace      string      `json:"namespace"`
	Ingress        string      `json:"ingress"`
	Service        string      `json:"service"`
	Pod            string      `json:"pod"`
}
