// Copyright © 2019 VMware
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
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic/dynamicinformer"

	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"

	"k8s.io/client-go/dynamic"

	"k8s.io/client-go/tools/cache"

	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/debug"
	cgrpc "github.com/projectcontour/contour/internal/grpc"
	"github.com/projectcontour/contour/internal/httpsvc"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/tools/leaderelection"
)

// registerServe registers the serve subcommand and flags
// with the Application provided.
func registerServe(app *kingpin.Application) (*kingpin.CmdClause, *serveContext) {
	serve := app.Command("serve", "Serve xDS API traffic.")

	// The precedence of configuration for contour serve is as follows:
	// config file, overridden by env vars, overridden by cli flags.
	// however, as -c is a cli flag, we don't know its valye til cli flags
	// have been parsed. To correct this ordering we assign a post parse
	// action to -c, then parse cli flags twice (see main.main). On the second
	// parse our action will return early, resulting in the precedence order
	// we want.
	var (
		configFile string
		parsed     bool
	)
	ctx := newServeContext()

	parseConfig := func(_ *kingpin.ParseContext) error {
		if parsed || configFile == "" {
			// if there is no config file supplied, or we've
			// already parsed it, return immediately.
			return nil
		}
		f, err := os.Open(configFile)
		if err != nil {
			return err
		}
		defer f.Close()
		dec := yaml.NewDecoder(f)
		parsed = true
		return dec.Decode(&ctx)
	}

	serve.Flag("config-path", "Path to base configuration.").Short('c').Action(parseConfig).ExistingFileVar(&configFile)

	serve.Flag("incluster", "Use in cluster configuration.").BoolVar(&ctx.InCluster)
	serve.Flag("kubeconfig", "Path to kubeconfig (if not in running inside a cluster).").StringVar(&ctx.Kubeconfig)

	serve.Flag("xds-address", "xDS gRPC API address.").StringVar(&ctx.xdsAddr)
	serve.Flag("xds-port", "xDS gRPC API port.").IntVar(&ctx.xdsPort)

	serve.Flag("stats-address", "Envoy /stats interface address.").StringVar(&ctx.statsAddr)
	serve.Flag("stats-port", "Envoy /stats interface port.").IntVar(&ctx.statsPort)

	serve.Flag("debug-http-address", "Address the debug http endpoint will bind to.").StringVar(&ctx.debugAddr)
	serve.Flag("debug-http-port", "Port the debug http endpoint will bind to.").IntVar(&ctx.debugPort)

	serve.Flag("http-address", "Address the metrics http endpoint will bind to.").StringVar(&ctx.metricsAddr)
	serve.Flag("http-port", "Port the metrics http endpoint will bind to.").IntVar(&ctx.metricsPort)

	serve.Flag("contour-cafile", "CA bundle file name for serving gRPC with TLS.").Envar("CONTOUR_CAFILE").StringVar(&ctx.caFile)
	serve.Flag("contour-cert-file", "Contour certificate file name for serving gRPC over TLS.").Envar("CONTOUR_CERT_FILE").StringVar(&ctx.contourCert)
	serve.Flag("contour-key-file", "Contour key file name for serving gRPC over TLS.").Envar("CONTOUR_KEY_FILE").StringVar(&ctx.contourKey)
	serve.Flag("insecure", "Allow serving without TLS secured gRPC.").BoolVar(&ctx.PermitInsecureGRPC)
	// TODO(sas) Deprecate `ingressroute-root-namespaces` in v1.0
	serve.Flag("ingressroute-root-namespaces", "DEPRECATED (Use 'root-namespaces'): Restrict contour to searching these namespaces for root ingress routes.").StringVar(&ctx.rootNamespaces)
	serve.Flag("root-namespaces", "Restrict contour to searching these namespaces for root ingress routes.").StringVar(&ctx.rootNamespaces)

	serve.Flag("ingress-class-name", "Contour IngressClass name.").StringVar(&ctx.ingressClass)

	serve.Flag("envoy-http-access-log", "Envoy HTTP access log.").StringVar(&ctx.httpAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log.").StringVar(&ctx.httpsAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests.").StringVar(&ctx.httpAddr)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests.").StringVar(&ctx.httpsAddr)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests.").IntVar(&ctx.httpPort)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests.").IntVar(&ctx.httpsPort)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners.").BoolVar(&ctx.useProxyProto)

	serve.Flag("accesslog-format", "Format for Envoy access logs.").StringVar(&ctx.AccessLogFormat)
	serve.Flag("disable-leader-election", "Disable leader election mechanism.").BoolVar(&ctx.DisableLeaderElection)

	serve.Flag("use-extensions-v1beta1-ingress", "Subscribe to the deprecated extensions/v1beta1.Ingress type.").BoolVar(&ctx.UseExtensionsV1beta1Ingress)
	return serve, ctx
}

// doServe runs the contour serve subcommand.
func doServe(log logrus.FieldLogger, ctx *serveContext) error {

	// step 0. get kube config
	config, err := restConfig(ctx.Kubeconfig, ctx.InCluster)
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// step 1. establish k8s core & dynamic client connections
	clients, err := newKubernetesClients(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic interface client: %w", err)
	}

	// step 2. create informers
	// note: 0 means resync timers are disabled
	coreInformers := coreinformers.NewSharedInformerFactory(clients.core, 0)
	dynamicInformers := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 0)

	// Create a set of SharedInformerFactories for each root-ingressroute namespace (if defined)
	namespacedInformers := map[string]coreinformers.SharedInformerFactory{}

	for _, namespace := range ctx.ingressRouteRootNamespaces() {
		if _, ok := namespacedInformers[namespace]; !ok {
			namespacedInformers[namespace] = coreinformers.NewSharedInformerFactoryWithOptions(
				clients.core, 0, coreinformers.WithNamespace(namespace))
		}
	}

	// setup prometheus registry and register base metrics.
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGoCollector())

	// step 3. build our mammoth Kubernetes event handler.
	eventHandler := &contour.EventHandler{
		CacheHandler: &contour.CacheHandler{
			ListenerVisitorConfig: contour.ListenerVisitorConfig{
				UseProxyProto:          ctx.useProxyProto,
				HTTPAddress:            ctx.httpAddr,
				HTTPPort:               ctx.httpPort,
				HTTPAccessLog:          ctx.httpAccessLog,
				HTTPSAddress:           ctx.httpsAddr,
				HTTPSPort:              ctx.httpsPort,
				HTTPSAccessLog:         ctx.httpsAccessLog,
				AccessLogType:          ctx.AccessLogFormat,
				AccessLogFields:        ctx.AccessLogFields,
				MinimumProtocolVersion: dag.MinProtoVersion(ctx.TLSConfig.MinimumProtocolVersion),
				RequestTimeout:         ctx.RequestTimeout,
			},
			ListenerCache: contour.NewListenerCache(ctx.statsAddr, ctx.statsPort),
			FieldLogger:   log.WithField("context", "CacheHandler"),
			Metrics:       metrics.NewMetrics(registry),
		},
		HoldoffDelay:    100 * time.Millisecond,
		HoldoffMaxDelay: 500 * time.Millisecond,
		StatusClient: &k8s.StatusWriter{
			Client: dynamicClient,
		},
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				RootNamespaces: ctx.ingressRouteRootNamespaces(),
				IngressClass:   ctx.ingressClass,
				FieldLogger:    log.WithField("context", "KubernetesCache"),
			},
			DisablePermitInsecure: ctx.DisablePermitInsecure,
		},
		FieldLogger: log.WithField("context", "contourEventHandler"),
	}

	// wrap eventHandler in an EventRecorder which tracks API server events.
	eventRecorder := &contour.EventRecorder{
		Next:    eventHandler,
		Counter: eventHandler.Metrics.EventHandlerOperations,
	}

	// wrap eventRecorder in a converter for objects from the dynamic client.
	dynamicHandler := &k8s.DynamicClientHandler{
		Next:      eventRecorder,
		Converter: k8s.NewUnstructuredConverter(),
		Logger:    log.WithField("context", "dynamicHandler"),
	}

	// step 4. register our resource event handler with the k8s informers.
	var informers []cache.SharedIndexInformer

	informers = registerEventHandler(informers, dynamicInformers.ForResource(ingressroutev1.IngressRouteGVR).Informer(), dynamicHandler)
	informers = registerEventHandler(informers, dynamicInformers.ForResource(ingressroutev1.TLSCertificateDelegationGVR).Informer(), dynamicHandler)
	informers = registerEventHandler(informers, dynamicInformers.ForResource(projectcontour.HTTPProxyGVR).Informer(), dynamicHandler)
	informers = registerEventHandler(informers, dynamicInformers.ForResource(projectcontour.TLSCertificateDelegationGVR).Informer(), dynamicHandler)
	informers = registerEventHandler(informers, coreInformers.Core().V1().Services().Informer(), eventRecorder)

	// After K8s 1.13 the API server will automatically translate extensions/v1beta1.Ingress objects
	// to networking/v1beta1.Ingress objects so we should only listen for one type or the other.
	// The default behavior is to listen for networking/v1beta1.Ingress objects and let the API server
	// transparently upgrade the extensions version for us.
	if ctx.UseExtensionsV1beta1Ingress {
		informers = registerEventHandler(informers, coreInformers.Extensions().V1beta1().Ingresses().Informer(), eventRecorder)
	} else {
		informers = registerEventHandler(informers, coreInformers.Networking().V1beta1().Ingresses().Informer(), eventRecorder)
	}

	// Add informers for each root-ingressroute namespaces
	for _, inf := range namespacedInformers {
		informers = registerEventHandler(informers, inf.Core().V1().Secrets().Informer(), eventRecorder)
	}

	// If root-ingressroutes are not defined, then add the informer for all namespaces
	if len(namespacedInformers) == 0 {
		informers = registerEventHandler(informers, coreInformers.Core().V1().Secrets().Informer(), eventRecorder)
	}

	// step 5. endpoints updates are handled directly by the EndpointsTranslator
	// due to their high update rate and their orthogonal nature.
	et := &contour.EndpointsTranslator{
		FieldLogger: log.WithField("context", "endpointstranslator"),
	}

	informers = registerEventHandler(informers, coreInformers.Core().V1().Endpoints().Informer(), et)

	// step 6. setup workgroup runner and register informers.
	var g workgroup.Group
	g.Add(startInformer(dynamicInformers, log.WithField("context", "contourinformers")))
	g.Add(startInformer(coreInformers, log.WithField("context", "coreinformers")))

	for ns, inf := range namespacedInformers {
		g.Add(startInformer(inf, log.WithField("context", "corenamespacedinformers").WithField("namespace", ns)))
	}

	// step 7. register our event handler with the workgroup
	g.Add(eventHandler.Start())

	// step 8. create metrics service and register with workgroup.
	metricsvc := metrics.Service{
		Service: httpsvc.Service{
			Addr:        ctx.metricsAddr,
			Port:        ctx.metricsPort,
			FieldLogger: log.WithField("context", "metricsvc"),
		},
		Client:   clients.core,
		Registry: registry,
	}
	g.Add(metricsvc.Start)

	// step 9. create debug service and register with workgroup.
	debugsvc := debug.Service{
		Service: httpsvc.Service{
			Addr:        ctx.debugAddr,
			Port:        ctx.debugPort,
			FieldLogger: log.WithField("context", "debugsvc"),
		},
		Builder: &eventHandler.Builder,
	}
	g.Add(debugsvc.Start)

	// step 10. if enabled, register leader election
	if !ctx.DisableLeaderElection {
		var le *leaderelection.LeaderElector
		var deposed chan struct{}
		le, eventHandler.IsLeader, deposed = newLeaderElector(log, ctx, clients.core, clients.coordination)

		g.AddContext(func(electionCtx context.Context) {
			log.WithFields(logrus.Fields{
				"configmapname":      ctx.LeaderElectionConfig.Name,
				"configmapnamespace": ctx.LeaderElectionConfig.Namespace,
			}).Info("started leader election")

			le.Run(electionCtx)
			log.Info("stopped leader election")
		})

		g.Add(func(stop <-chan struct{}) error {
			log := log.WithField("context", "leaderelection-elected")
			leader := eventHandler.IsLeader
			for {
				select {
				case <-stop:
					// shut down
					log.Info("stopped leader election")
					return nil
				case <-leader:
					log.Info("elected as leader, triggering rebuild")
					eventHandler.UpdateNow()

					// disable this case
					leader = nil
				}
			}
		})

		g.Add(func(stop <-chan struct{}) error {
			// If we get deposed as leader, shut it down.
			log := log.WithField("context", "leaderelection-deposer")
			select {
			case <-stop:
				// shut down
				log.Info("stopped leader election")
			case <-deposed:
				log.Info("deposed as leader, shutting down")
			}
			return nil
		})
	} else {
		log.Info("Leader election disabled")

		// leadership election disabled, hardwire IsLeader to be always readable.
		leader := make(chan struct{})
		close(leader)
		eventHandler.IsLeader = leader
	}

	// step 12. create grpc handler and register with workgroup.
	g.Add(func(stop <-chan struct{}) error {
		log := log.WithField("context", "grpc")

		synced := make([]cache.InformerSynced, 0, len(informers))
		for _, inf := range informers {
			synced = append(synced, inf.HasSynced)
		}

		log.Printf("waiting for informer caches to sync")
		if !cache.WaitForCacheSync(stop, synced...) {
			return fmt.Errorf("error waiting for cache to sync")
		}
		log.Printf("informer caches synced")

		resources := map[string]cgrpc.Resource{
			eventHandler.CacheHandler.ClusterCache.TypeURL():  &eventHandler.CacheHandler.ClusterCache,
			eventHandler.CacheHandler.RouteCache.TypeURL():    &eventHandler.CacheHandler.RouteCache,
			eventHandler.CacheHandler.ListenerCache.TypeURL(): &eventHandler.CacheHandler.ListenerCache,
			eventHandler.CacheHandler.SecretCache.TypeURL():   &eventHandler.CacheHandler.SecretCache,
			et.TypeURL(): et,
		}
		opts := ctx.grpcOptions()
		s := cgrpc.NewAPI(log, resources, registry, opts...)
		addr := net.JoinHostPort(ctx.xdsAddr, strconv.Itoa(ctx.xdsPort))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}

		log = log.WithField("address", addr)
		if ctx.PermitInsecureGRPC {
			log = log.WithField("insecure", true)
		}

		log.Info("started xDS server")
		defer log.Info("stopped xDS server")

		go func() {
			<-stop
			s.Stop()
		}()

		return s.Serve(l)
	})

	// step 13. Setup SIGTERM handler
	g.Add(func(stop <-chan struct{}) error {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM)
		select {
		case <-c:
			log.WithField("context", "sigterm-handler").Info("received SIGTERM, shutting down")
		case <-stop:
			// Do nothing. The group is shutting down.
		}
		return nil
	})

	// step 14. GO!
	return g.Run()
}

func registerEventHandler(informers []cache.SharedIndexInformer, inf cache.SharedIndexInformer, handler cache.ResourceEventHandler) []cache.SharedIndexInformer {
	inf.AddEventHandler(handler)
	return append(informers, inf)
}

type informer interface {
	Start(stopCh <-chan struct{})
}

func startInformer(inf informer, log logrus.FieldLogger) func(stop <-chan struct{}) error {
	return func(stop <-chan struct{}) error {
		log.Println("started informer")
		defer log.Println("stopped informer")
		inf.Start(stop)
		<-stop
		return nil
	}
}
