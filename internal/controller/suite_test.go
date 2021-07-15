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

package controller

import (
	"context"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/bombsimon/logrusr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/projectcontour/contour/internal/xds"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	gateway_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const gatewayClassControllerName = "test.io/contour"

var (
	cl       client.Client
	mgr      ctrl.Manager
	testEnv  *envtest.Environment
	timeout  = time.Second * 10
	interval = time.Second * 1
)

const (
	statsAddress = "0.0.0.0"
	statsPort    = 8002
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Runtime Suite")
}

var _ = BeforeSuite(func() {

	var g workgroup.Group

	log := logrus.New()
	log.Out = GinkgoWriter
	log.Level = logrus.DebugLevel
	logf.SetLogger(logrusr.NewLogger(log))

	By("Bootstrapping the test environment")
	gatewayCRDs := filepath.Join("..", "..", "examples", "gateway", "00-crds.yaml")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{gatewayCRDs},
	}

	// Start the test environment which spins up a kubernetes api.
	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = scheme.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = gateway_v1alpha1.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	et := xdscache_v3.NewEndpointsTranslator(log)
	listenerConfig := xdscache_v3.ListenerConfig{}

	resources := []xdscache.ResourceCache{
		xdscache_v3.NewListenerCache(listenerConfig, statsAddress, statsPort),
		&xdscache_v3.SecretCache{},
		&xdscache_v3.RouteCache{},
		&xdscache_v3.ClusterCache{},
		et,
	}

	registry := prometheus.NewRegistry()
	converter, err := k8s.NewUnstructuredConverter()
	Expect(err).NotTo(HaveOccurred())

	// Create an event handler to take events from the controllers
	// and process to validate status.
	eh := &contour.EventHandler{
		IsLeader:    make(chan struct{}),
		FieldLogger: log,
		Sequence:    make(chan int, 1),
		//nolint:gosec
		HoldoffDelay: time.Duration(rand.Intn(100)) * time.Millisecond,
		//nolint:gosec
		HoldoffMaxDelay: time.Duration(rand.Intn(500)) * time.Millisecond,
		Observer: &contour.RebuildMetricsObserver{
			Metrics:      metrics.NewMetrics(registry),
			NextObserver: dag.ComposeObservers(xdscache.ObserversOf(resources)...),
		},
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				FieldLogger: log,
				ConfiguredGateway: types.NamespacedName{
					Name:      "contour",
					Namespace: "projectcontour",
				},
			},
		},
	}

	// Setup a Manager
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	_, err = NewGatewayController(mgr, eh, log, gatewayClassControllerName)
	Expect(err).ToNot(HaveOccurred())

	_, err = NewGatewayClassController(mgr, eh, log, gatewayClassControllerName)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err := mgr.Start(signals.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	// Get Kubernetes client from Manager.
	cl = mgr.GetClient()
	Expect(cl).ToNot(BeNil())

	// Create a k8s.Clients struct to pass off to the StatusUpdateHandler.
	clients, err := k8s.NewClientFromRestConfig(mgr.GetConfig())
	Expect(err).ToNot(HaveOccurred())

	sh := k8s.StatusUpdateHandler{
		Log:           log.WithField("context", "StatusUpdateHandler"),
		Clients:       clients,
		LeaderElected: eh.IsLeader,
		Converter:     converter,
	}

	eh.Builder.Processors = []dag.Processor{
		&dag.IngressProcessor{
			FieldLogger: log.WithField("context", "IngressProcessor"),
		},
		&dag.ExtensionServiceProcessor{
			FieldLogger: log.WithField("context", "ExtensionServiceProcessor"),
		},
		&dag.HTTPProxyProcessor{},
		&dag.GatewayAPIProcessor{
			FieldLogger: log.WithField("context", "GatewayAPIProcessor"),
		},
		&dag.ListenerProcessor{},
	}

	// Now we have the statusUpdateHandler, we can create the event handler's StatusUpdater, which will take the
	// status updates from the DAG, and send them to the status update handler.
	eh.StatusUpdater = sh.Writer()

	g.AddContext(func(taskCtx context.Context) error {
		err := clients.StartInformers(taskCtx)
		Expect(err).ToNot(HaveOccurred())
		<-taskCtx.Done()
		return nil
	})
	g.Add(sh.Start)

	// Make this event handler win the leader election.
	close(eh.IsLeader)

	srv := xds.NewServer(registry)
	contour_xds_v3.RegisterServer(contour_xds_v3.NewContourServer(log, xdscache.ResourcesOf(resources)...), srv)

	// Start the EventHandler.
	g.Add(eh.Start())

	done := make(chan error)
	go func() {
		done <- g.Run(context.Background())
	}()

	// Create test namespaces
	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "projectcontour",
		},
	}
	Expect(cl.Create(context.Background(), namespace)).Should(Succeed())
})

var _ = AfterSuite(func() {
	By("Expecting the test environment teardown to complete")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

// isGatewayClassAdmitted returns true if gc status is "Admitted=true".
func isGatewayClassAdmitted(gc *gateway_v1alpha1.GatewayClass) bool {
	for _, c := range gc.Status.Conditions {
		if c.Type == string(gateway_v1alpha1.GatewayClassConditionStatusAdmitted) &&
			c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// isGatewayReady returns true if gc status is "Ready=true".
func isGatewayReady(gateway *gateway_v1alpha1.Gateway) bool {
	for _, c := range gateway.Status.Conditions {
		if c.Type == string(gateway_v1alpha1.GatewayConditionReady) &&
			c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// StubEventHandler fills the interface that the EventHandler
// is used for since the Controller tests do not require
// the event handler for its tests.
type StubEventHandler struct {
}

func (e *StubEventHandler) OnAdd(obj interface{}) {
}

func (e *StubEventHandler) OnUpdate(oldObj, newObj interface{}) {
}

func (e *StubEventHandler) OnDelete(obj interface{}) {
}
