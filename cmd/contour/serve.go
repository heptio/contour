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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/projectcontour/contour/internal/controller"

	envoy_server_v3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/debug"
	"github.com/projectcontour/contour/internal/health"
	"github.com/projectcontour/contour/internal/httpsvc"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/projectcontour/contour/internal/xds"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	controller_config "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// Add RBAC policy to support leader election.
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=create;get;update

// Add RBAC policy to support getting CRDs.
// +kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=list

// registerServe registers the serve subcommand and flags
// with the Application provided.
func registerServe(app *kingpin.Application) (*kingpin.CmdClause, *serveContext) {
	serve := app.Command("serve", "Serve xDS API traffic.")

	// The precedence of configuration for contour serve is as follows:
	// config file, overridden by env vars, overridden by cli flags.
	// however, as -c is a cli flag, we don't know its value til cli flags
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

		params, err := config.Parse(f)
		if err != nil {
			return err
		}

		if err := params.Validate(); err != nil {
			return fmt.Errorf("invalid Contour configuration: %w", err)
		}

		parsed = true
		ctx.Config = *params

		return nil
	}

	serve.Flag("config-path", "Path to base configuration.").Short('c').PlaceHolder("/path/to/file").Action(parseConfig).ExistingFileVar(&configFile)

	serve.Flag("incluster", "Use in cluster configuration.").BoolVar(&ctx.Config.InCluster)
	serve.Flag("kubeconfig", "Path to kubeconfig (if not in running inside a cluster).").PlaceHolder("/path/to/file").StringVar(&ctx.Config.Kubeconfig)

	serve.Flag("xds-address", "xDS gRPC API address.").PlaceHolder("<ipaddr>").StringVar(&ctx.xdsAddr)
	serve.Flag("xds-port", "xDS gRPC API port.").PlaceHolder("<port>").IntVar(&ctx.xdsPort)

	serve.Flag("stats-address", "Envoy /stats interface address.").PlaceHolder("<ipaddr>").StringVar(&ctx.statsAddr)
	serve.Flag("stats-port", "Envoy /stats interface port.").PlaceHolder("<port>").IntVar(&ctx.statsPort)

	serve.Flag("debug-http-address", "Address the debug http endpoint will bind to.").PlaceHolder("<ipaddr>").StringVar(&ctx.debugAddr)
	serve.Flag("debug-http-port", "Port the debug http endpoint will bind to.").PlaceHolder("<port>").IntVar(&ctx.debugPort)

	serve.Flag("http-address", "Address the metrics HTTP endpoint will bind to.").PlaceHolder("<ipaddr>").StringVar(&ctx.metricsAddr)
	serve.Flag("http-port", "Port the metrics HTTP endpoint will bind to.").PlaceHolder("<port>").IntVar(&ctx.metricsPort)
	serve.Flag("health-address", "Address the health HTTP endpoint will bind to.").PlaceHolder("<ipaddr>").StringVar(&ctx.healthAddr)
	serve.Flag("health-port", "Port the health HTTP endpoint will bind to.").PlaceHolder("<port>").IntVar(&ctx.healthPort)

	serve.Flag("contour-cafile", "CA bundle file name for serving gRPC with TLS.").Envar("CONTOUR_CAFILE").StringVar(&ctx.caFile)
	serve.Flag("contour-cert-file", "Contour certificate file name for serving gRPC over TLS.").PlaceHolder("/path/to/file").Envar("CONTOUR_CERT_FILE").StringVar(&ctx.contourCert)
	serve.Flag("contour-key-file", "Contour key file name for serving gRPC over TLS.").PlaceHolder("/path/to/file").Envar("CONTOUR_KEY_FILE").StringVar(&ctx.contourKey)
	serve.Flag("insecure", "Allow serving without TLS secured gRPC.").BoolVar(&ctx.PermitInsecureGRPC)
	serve.Flag("root-namespaces", "Restrict contour to searching these namespaces for root ingress routes.").PlaceHolder("<ns,ns>").StringVar(&ctx.rootNamespaces)

	serve.Flag("ingress-class-name", "Contour IngressClass name.").PlaceHolder("<name>").StringVar(&ctx.ingressClassName)
	serve.Flag("ingress-status-address", "Address to set in Ingress object status.").PlaceHolder("<address>").StringVar(&ctx.Config.IngressStatusAddress)
	serve.Flag("envoy-http-access-log", "Envoy HTTP access log.").PlaceHolder("/path/to/file").StringVar(&ctx.httpAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log.").PlaceHolder("/path/to/file").StringVar(&ctx.httpsAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests.").PlaceHolder("<ipaddr>").StringVar(&ctx.httpAddr)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests.").PlaceHolder("<ipaddr>").StringVar(&ctx.httpsAddr)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests.").PlaceHolder("<port>").IntVar(&ctx.httpPort)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests.").PlaceHolder("<port>").IntVar(&ctx.httpsPort)
	serve.Flag("envoy-service-name", "Name of the Envoy service to inspect for Ingress status details.").PlaceHolder("<name>").StringVar(&ctx.Config.EnvoyServiceName)
	serve.Flag("envoy-service-namespace", "Envoy Service Namespace.").PlaceHolder("<namespace>").StringVar(&ctx.Config.EnvoyServiceNamespace)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners.").BoolVar(&ctx.useProxyProto)

	serve.Flag("accesslog-format", "Format for Envoy access logs.").PlaceHolder("<envoy|json>").StringVar((*string)(&ctx.Config.AccessLogFormat))
	serve.Flag("disable-leader-election", "Disable leader election mechanism.").BoolVar(&ctx.DisableLeaderElection)

	serve.Flag("debug", "Enable debug logging.").Short('d').BoolVar(&ctx.Config.Debug)
	serve.Flag("kubernetes-debug", "Enable Kubernetes client debug logging with log level.").PlaceHolder("<log level>").UintVar(&ctx.KubernetesDebug)
	return serve, ctx
}

// validateCRDs inspects all CRDs in the projectcontour.io group and logs a warning
// if they have spec.preserveUnknownFields set to true, since this indicates that they
// were created as v1beta1 and the user has not upgraded them to be fully v1-compatible.
func validateCRDs(dynamicClient dynamic.Interface, log logrus.FieldLogger) {
	client := dynamicClient.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})

	crds, err := client.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Warnf("error listing v1 custom resource definitions: %v", err)
		return
	}

	for _, crd := range crds.Items {
		log = log.WithField("crd", crd.GetName())

		if group, _, _ := unstructured.NestedString(crd.Object, "spec", "group"); group != contour_api_v1.GroupName {
			log.Debugf("CRD is not in projectcontour.io API group, ignoring")
			continue
		}

		preserveUnknownFields, found, err := unstructured.NestedBool(crd.Object, "spec", "preserveUnknownFields")
		if err != nil {
			log.Warnf("error getting CRD's spec.preserveUnknownFields value: %v", err)
			continue
		}
		if found && preserveUnknownFields {
			log.Warnf("CRD was created as v1beta1 since it has spec.preserveUnknownFields set to true; it should be upgraded to v1 per https://projectcontour.io/resources/upgrading/")
			continue
		}

		log.Debugf("CRD is fully v1-compatible since it has spec.preserveUnknownFields set to false")
	}
}

// doServe runs the contour serve subcommand.
func doServe(log logrus.FieldLogger, ctx *serveContext) error {
	// Establish k8s core & dynamic client connections.
	clients, err := k8s.NewClients(ctx.Config.Kubeconfig, ctx.Config.InCluster)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	// Validate that Contour CRDs have been updated to v1.
	validateCRDs(clients.DynamicClient(), log)

	// informerNamespaces is a list of namespaces that we should start informers for.
	var informerNamespaces []string

	fallbackCert := namespacedNameOf(ctx.Config.TLS.FallbackCertificate)
	clientCert := namespacedNameOf(ctx.Config.TLS.ClientCertificate)

	if rootNamespaces := ctx.proxyRootNamespaces(); len(rootNamespaces) > 0 {
		informerNamespaces = append(informerNamespaces, rootNamespaces...)

		// Add the FallbackCertificateNamespace to informerNamespaces if it isn't present.
		if !contains(informerNamespaces, ctx.Config.TLS.FallbackCertificate.Namespace) && fallbackCert != nil {
			informerNamespaces = append(informerNamespaces, ctx.Config.TLS.FallbackCertificate.Namespace)
			log.WithField("context", "fallback-certificate").
				Infof("fallback certificate namespace %q not defined in 'root-namespaces', adding namespace to watch",
					ctx.Config.TLS.FallbackCertificate.Namespace)
		}

		// Add the client certificate namespace to informerNamespaces if it isn't present.
		if !contains(informerNamespaces, ctx.Config.TLS.ClientCertificate.Namespace) && clientCert != nil {
			informerNamespaces = append(informerNamespaces, ctx.Config.TLS.ClientCertificate.Namespace)
			log.WithField("context", "envoy-client-certificate").
				Infof("client certificate namespace %q not defined in 'root-namespaces', adding namespace to watch",
					ctx.Config.TLS.ClientCertificate.Namespace)
		}
	}

	// Set up Prometheus registry and register base metrics.
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGoCollector())

	// Before we can build the event handler, we need to initialize the converter we'll
	// use to convert from Unstructured.
	converter, err := k8s.NewUnstructuredConverter()
	if err != nil {
		return err
	}

	// XXX(jpeach) we know the config file validated, so all
	// the timeouts will parse. Shall we add a `timeout.MustParse()`
	// and use it here?

	connectionIdleTimeout, err := timeout.Parse(ctx.Config.Timeouts.ConnectionIdleTimeout)
	if err != nil {
		return fmt.Errorf("error parsing connection idle timeout: %w", err)
	}
	streamIdleTimeout, err := timeout.Parse(ctx.Config.Timeouts.StreamIdleTimeout)
	if err != nil {
		return fmt.Errorf("error parsing stream idle timeout: %w", err)
	}
	delayedCloseTimeout, err := timeout.Parse(ctx.Config.Timeouts.DelayedCloseTimeout)
	if err != nil {
		return fmt.Errorf("error parsing delayed close timeout: %w", err)
	}
	maxConnectionDuration, err := timeout.Parse(ctx.Config.Timeouts.MaxConnectionDuration)
	if err != nil {
		return fmt.Errorf("error parsing max connection duration: %w", err)
	}
	connectionShutdownGracePeriod, err := timeout.Parse(ctx.Config.Timeouts.ConnectionShutdownGracePeriod)
	if err != nil {
		return fmt.Errorf("error parsing connection shutdown grace period: %w", err)
	}
	requestTimeout, err := timeout.Parse(ctx.Config.Timeouts.RequestTimeout)
	if err != nil {
		return fmt.Errorf("error parsing request timeout: %w", err)
	}

	// connection balancer
	if ok := ctx.Config.Listener.ConnectionBalancer == "exact" || ctx.Config.Listener.ConnectionBalancer == ""; !ok {
		log.Warnf("Invalid listener connection balancer value %q. Only 'exact' connection balancing is supported for now.", ctx.Config.Listener.ConnectionBalancer)
		ctx.Config.Listener.ConnectionBalancer = ""
	}

	listenerConfig := xdscache_v3.ListenerConfig{
		UseProxyProto: ctx.useProxyProto,
		HTTPListeners: map[string]xdscache_v3.Listener{
			"ingress_http": {
				Name:    "ingress_http",
				Address: ctx.httpAddr,
				Port:    ctx.httpPort,
			},
		},
		HTTPSListeners: map[string]xdscache_v3.Listener{
			"ingress_https": {
				Name:    "ingress_https",
				Address: ctx.httpsAddr,
				Port:    ctx.httpsPort,
			},
		},
		HTTPAccessLog:                 ctx.httpAccessLog,
		HTTPSAccessLog:                ctx.httpsAccessLog,
		AccessLogType:                 ctx.Config.AccessLogFormat,
		AccessLogFields:               ctx.Config.AccessLogFields,
		MinimumTLSVersion:             annotation.MinTLSVersion(ctx.Config.TLS.MinimumProtocolVersion, "1.2"),
		CipherSuites:                  config.SanitizeCipherSuites(ctx.Config.TLS.CipherSuites),
		RequestTimeout:                requestTimeout,
		ConnectionIdleTimeout:         connectionIdleTimeout,
		StreamIdleTimeout:             streamIdleTimeout,
		DelayedCloseTimeout:           delayedCloseTimeout,
		MaxConnectionDuration:         maxConnectionDuration,
		ConnectionShutdownGracePeriod: connectionShutdownGracePeriod,
		DefaultHTTPVersions:           parseDefaultHTTPVersions(ctx.Config.DefaultHTTPVersions),
		AllowChunkedLength:            !ctx.Config.DisableAllowChunkedLength,
		XffNumTrustedHops:             ctx.Config.Network.XffNumTrustedHops,
		ConnectionBalancer:            ctx.Config.Listener.ConnectionBalancer,
	}

	if ctx.Config.RateLimitService.ExtensionService != "" {
		namespacedName := k8s.NamespacedNameFrom(ctx.Config.RateLimitService.ExtensionService)
		client := clients.DynamicClient().Resource(contour_api_v1alpha1.ExtensionServiceGVR).Namespace(namespacedName.Namespace)

		// ensure the specified ExtensionService exists
		res, err := client.Get(context.Background(), namespacedName.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error getting rate limit extension service %s: %v", namespacedName, err)
		}
		var extensionSvc contour_api_v1alpha1.ExtensionService
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, &extensionSvc); err != nil {
			return fmt.Errorf("error converting rate limit extension service %s: %v", namespacedName, err)
		}
		// get the response timeout from the ExtensionService
		var responseTimeout timeout.Setting
		if tp := extensionSvc.Spec.TimeoutPolicy; tp != nil {
			responseTimeout, err = timeout.Parse(tp.Response)
			if err != nil {
				return fmt.Errorf("error parsing rate limit extension service %s response timeout: %v", namespacedName, err)
			}
		}

		listenerConfig.RateLimitConfig = &xdscache_v3.RateLimitConfig{
			ExtensionService:        namespacedName,
			Domain:                  ctx.Config.RateLimitService.Domain,
			Timeout:                 responseTimeout,
			FailOpen:                ctx.Config.RateLimitService.FailOpen,
			EnableXRateLimitHeaders: ctx.Config.RateLimitService.EnableXRateLimitHeaders,
		}
	}

	contourMetrics := metrics.NewMetrics(registry)

	// Endpoints updates are handled directly by the EndpointsTranslator
	// due to their high update rate and their orthogonal nature.
	endpointHandler := xdscache_v3.NewEndpointsTranslator(log.WithField("context", "endpointstranslator"))

	resources := []xdscache.ResourceCache{
		xdscache_v3.NewListenerCache(listenerConfig, ctx.statsAddr, ctx.statsPort),
		&xdscache_v3.SecretCache{},
		&xdscache_v3.RouteCache{},
		&xdscache_v3.ClusterCache{},
		endpointHandler,
	}

	// snapshotHandler is used to produce new snapshots when the internal state changes for any xDS resource.
	snapshotHandler := xdscache.NewSnapshotHandler(resources, log.WithField("context", "snapshotHandler"))

	// register observer for endpoints updates.
	endpointHandler.Observer = contour.ComposeObservers(snapshotHandler)

	// Log that we're using the fallback certificate if configured.
	if fallbackCert != nil {
		log.WithField("context", "fallback-certificate").Infof("enabled fallback certificate with secret: %q", fallbackCert)
	}
	if clientCert != nil {
		log.WithField("context", "envoy-client-certificate").Infof("enabled client certificate with secret: %q", clientCert)
	}

	// Build the core Kubernetes event handler.
	eventHandler := &contour.EventHandler{
		HoldoffDelay:    100 * time.Millisecond,
		HoldoffMaxDelay: 500 * time.Millisecond,
		Observer:        dag.ComposeObservers(append(xdscache.ObserversOf(resources), snapshotHandler)...),
		Builder:         getDAGBuilder(ctx, clients, clientCert, fallbackCert, log),
		FieldLogger:     log.WithField("context", "contourEventHandler"),
	}

	// Wrap eventHandler in a converter for objects from the dynamic client.
	// and an EventRecorder which tracks API server events.
	dynamicHandler := k8s.DynamicClientHandler{
		Next: &contour.EventRecorder{
			Next:    eventHandler,
			Counter: contourMetrics.EventHandlerOperations,
		},
		Converter: converter,
		Logger:    log.WithField("context", "dynamicHandler"),
	}

	// Inform on DefaultResources.
	for _, r := range k8s.DefaultResources() {
		inf, err := clients.InformerForResource(r)
		if err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}

		inf.AddEventHandler(&dynamicHandler)
	}

	for _, r := range k8s.IngressV1Resources() {
		if err := informOnResource(clients, r, &dynamicHandler); err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Set up workgroup runner and register informers.
	var g workgroup.Group

	// Only inform on Gateway API resources if Gateway API is found.
	if ctx.Config.GatewayConfig != nil {
		if clients.ResourcesExist(k8s.GatewayAPIResources()...) {

			// Setup a Manager
			mgr, err := manager.New(controller_config.GetConfigOrDie(), manager.Options{})
			if err != nil {
				log.WithError(err).Fatal("unable to set up controller manager")
			}

			// Add the Gateway API Scheme.
			err = gatewayapi_v1alpha1.AddToScheme(mgr.GetScheme())
			if err != nil {
				log.WithError(err).Fatal("unable to add Gateway API to scheme.")
			}

			// Create and register the gatewayclass controller with the manager.
			gatewayClassControllerName := ctx.Config.GatewayConfig.ControllerName
			if _, err := controller.NewGatewayClassController(mgr, &dynamicHandler,
				log.WithField("context", "gatewayclass-controller"), gatewayClassControllerName); err != nil {
				log.WithError(err).Fatal("failed to create gatewayclass-controller")
			}

			// Create and register the NewGatewayController controller with the manager.
			if _, err := controller.NewGatewayController(mgr, &dynamicHandler,
				log.WithField("context", "gateway-controller"), gatewayClassControllerName); err != nil {
				log.WithError(err).Fatal("failed to create gateway-controller")
			}

			// Create and register the NewHTTPRouteController controller with the manager.
			if _, err := controller.NewHTTPRouteController(mgr, &dynamicHandler, log.WithField("context", "httproute-controller")); err != nil {
				log.WithError(err).Fatal("failed to create httproute-controller")
			}

			// Create and register the NewTLSRouteController controller with the manager.
			if _, err := controller.NewTLSRouteController(mgr, &dynamicHandler, log.WithField("context", "tlsroute-controller")); err != nil {
				log.WithError(err).Fatal("failed to create tlsroute-controller")
			}

			// Inform on Namespaces.
			if err := informOnResource(clients, k8s.NamespacesResource(), &dynamicHandler); err != nil {
				log.WithError(err).WithField("resource", k8s.NamespacesResource()).Fatal("failed to create informer")
			}

			// Start Manager
			g.AddContext(func(taskCtx context.Context) error {
				return mgr.Start(signals.SetupSignalHandler())
			})
		} else {
			log.Fatalf("Gateway API Gateway configured but APIs not installed in cluster.")
		}
	}

	// Inform on secrets, filtering by root namespaces.
	for _, r := range k8s.SecretsResources() {
		var handler cache.ResourceEventHandler = &dynamicHandler

		// If root namespaces are defined, filter for secrets in only those namespaces.
		if len(informerNamespaces) > 0 {
			handler = k8s.NewNamespaceFilter(informerNamespaces, &dynamicHandler)
		}

		if err := informOnResource(clients, r, handler); err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Inform on endpoints.
	for _, r := range k8s.EndpointsResources() {
		if err := informOnResource(clients, r, &k8s.DynamicClientHandler{
			Next: &contour.EventRecorder{
				Next:    endpointHandler,
				Counter: contourMetrics.EventHandlerOperations,
			},
			Converter: converter,
			Logger:    log.WithField("context", "endpointstranslator"),
		}); err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Register a task to start all the informers.
	g.AddContext(func(taskCtx context.Context) error {
		log := log.WithField("context", "informers")

		log.Info("starting informers")
		defer log.Println("stopped informers")

		if err := clients.StartInformers(taskCtx); err != nil {
			log.WithError(err).Error("failed to start informers")
		}

		<-taskCtx.Done()
		return nil
	})

	// Register our event handler with the workgroup.
	g.Add(eventHandler.Start())

	// Create metrics service and register with workgroup.
	metricsvc := httpsvc.Service{
		Addr:        ctx.metricsAddr,
		Port:        ctx.metricsPort,
		FieldLogger: log.WithField("context", "metricsvc"),
		ServeMux:    http.ServeMux{},
	}

	metricsvc.ServeMux.Handle("/metrics", metrics.Handler(registry))

	if ctx.healthAddr == ctx.metricsAddr && ctx.healthPort == ctx.metricsPort {
		h := health.Handler(clients.ClientSet())
		metricsvc.ServeMux.Handle("/health", h)
		metricsvc.ServeMux.Handle("/healthz", h)
	}

	g.Add(metricsvc.Start)

	// Create a separate health service if required.
	if ctx.healthAddr != ctx.metricsAddr || ctx.healthPort != ctx.metricsPort {
		healthsvc := httpsvc.Service{
			Addr:        ctx.healthAddr,
			Port:        ctx.healthPort,
			FieldLogger: log.WithField("context", "healthsvc"),
		}

		h := health.Handler(clients.ClientSet())
		healthsvc.ServeMux.Handle("/health", h)
		healthsvc.ServeMux.Handle("/healthz", h)

		g.Add(healthsvc.Start)
	}

	// Create debug service and register with workgroup.
	debugsvc := debug.Service{
		Service: httpsvc.Service{
			Addr:        ctx.debugAddr,
			Port:        ctx.debugPort,
			FieldLogger: log.WithField("context", "debugsvc"),
		},
		Builder: &eventHandler.Builder,
	}
	g.Add(debugsvc.Start)

	// Register leadership election.
	if ctx.DisableLeaderElection {
		eventHandler.IsLeader = disableLeaderElection(log)
	} else {
		eventHandler.IsLeader = setupLeadershipElection(&g, log, &ctx.Config.LeaderElection, clients, eventHandler.UpdateNow)
	}

	// Once we have the leadership detection channel, we can
	// push DAG rebuild metrics onto the observer stack.
	eventHandler.Observer = &contour.RebuildMetricsObserver{
		Metrics:      contourMetrics,
		IsLeader:     eventHandler.IsLeader,
		NextObserver: eventHandler.Observer,
	}

	sh := k8s.StatusUpdateHandler{
		Log:           log.WithField("context", "StatusUpdateHandler"),
		Clients:       clients,
		LeaderElected: eventHandler.IsLeader,
		Converter:     converter,
	}
	g.Add(sh.Start)

	// Now we have the statusUpdateHandler, we can create the event handler's StatusUpdater, which will take the
	// status updates from the DAG, and send them to the status update handler.
	eventHandler.StatusUpdater = sh.Writer()

	// Set up ingress load balancer status writer.
	lbsw := loadBalancerStatusWriter{
		log:              log.WithField("context", "loadBalancerStatusWriter"),
		clients:          clients,
		isLeader:         eventHandler.IsLeader,
		lbStatus:         make(chan corev1.LoadBalancerStatus, 1),
		ingressClassName: ctx.ingressClassName,
		statusUpdater:    sh.Writer(),
		Converter:        converter,
	}
	g.Add(lbsw.Start)

	// Register an informer to watch envoy's service if we haven't been given static details.
	if lbAddr := ctx.Config.IngressStatusAddress; lbAddr != "" {
		log.WithField("loadbalancer-address", lbAddr).Info("Using supplied information for Ingress status")
		lbsw.lbStatus <- parseStatusFlag(lbAddr)
	} else {
		dynamicServiceHandler := k8s.DynamicClientHandler{
			Next: &k8s.ServiceStatusLoadBalancerWatcher{
				ServiceName: ctx.Config.EnvoyServiceName,
				LBStatus:    lbsw.lbStatus,
				Log:         log.WithField("context", "serviceStatusLoadBalancerWatcher"),
			},
			Converter: converter,
			Logger:    log.WithField("context", "serviceStatusLoadBalancerWatcher"),
		}

		for _, r := range k8s.ServicesResources() {
			var handler cache.ResourceEventHandler = &dynamicServiceHandler

			if ctx.Config.EnvoyServiceNamespace != "" {
				handler = k8s.NewNamespaceFilter([]string{ctx.Config.EnvoyServiceNamespace}, handler)
			}

			if err := informOnResource(clients, r, handler); err != nil {
				log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
			}
		}

		log.WithField("envoy-service-name", ctx.Config.EnvoyServiceName).
			WithField("envoy-service-namespace", ctx.Config.EnvoyServiceNamespace).
			Info("Watching Service for Ingress status")
	}

	g.AddContext(func(taskCtx context.Context) error {
		log := log.WithField("context", "xds")

		log.Printf("waiting for informer caches to sync")
		if !clients.WaitForCacheSync(taskCtx) {
			return errors.New("informer cache failed to sync")
		}
		log.Printf("informer caches synced")

		grpcServer := xds.NewServer(registry, ctx.grpcOptions(log)...)

		switch ctx.Config.Server.XDSServerType {
		case config.EnvoyServerType:
			v3cache := contour_xds_v3.NewSnapshotCache(false, log)
			snapshotHandler.AddSnapshotter(v3cache)
			contour_xds_v3.RegisterServer(envoy_server_v3.NewServer(taskCtx, v3cache, contour_xds_v3.NewRequestLoggingCallbacks(log)), grpcServer)
		case config.ContourServerType:
			contour_xds_v3.RegisterServer(contour_xds_v3.NewContourServer(log, xdscache.ResourcesOf(resources)...), grpcServer)
		default:
			// This can't happen due to config validation.
			log.Fatalf("invalid xDS server type %q", ctx.Config.Server.XDSServerType)
		}

		addr := net.JoinHostPort(ctx.xdsAddr, strconv.Itoa(ctx.xdsPort))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}

		log = log.WithField("address", addr)
		if ctx.PermitInsecureGRPC {
			log = log.WithField("insecure", true)
		}

		log.Infof("started xDS server type: %q", ctx.Config.Server.XDSServerType)
		defer log.Info("stopped xDS server")

		go func() {
			<-taskCtx.Done()

			// We don't use GracefulStop here because envoy
			// has long-lived hanging xDS requests. There's no
			// mechanism to make those pending requests fail,
			// so we forcibly terminate the TCP sessions.
			grpcServer.Stop()
		}()

		return grpcServer.Serve(l)
	})

	// Set up SIGTERM handler for graceful shutdown.
	g.Add(func(stop <-chan struct{}) error {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		select {
		case sig := <-c:
			log.WithField("context", "sigterm-handler").WithField("signal", sig).Info("shutting down")
		case <-stop:
			// Do nothing. The group is shutting down.
		}
		return nil
	})

	// GO!
	return g.Run(context.Background())
}

func getDAGBuilder(ctx *serveContext, clients *k8s.Clients, clientCert, fallbackCert *types.NamespacedName, log logrus.FieldLogger) dag.Builder {
	var requestHeadersPolicy dag.HeadersPolicy
	if ctx.Config.Policy.RequestHeadersPolicy.Set != nil {
		requestHeadersPolicy.Set = make(map[string]string)
		for k, v := range ctx.Config.Policy.RequestHeadersPolicy.Set {
			requestHeadersPolicy.Set[k] = v
		}
	}
	if ctx.Config.Policy.RequestHeadersPolicy.Remove != nil {
		requestHeadersPolicy.Remove = make([]string, 0, len(ctx.Config.Policy.RequestHeadersPolicy.Remove))
		requestHeadersPolicy.Remove = append(requestHeadersPolicy.Remove, ctx.Config.Policy.RequestHeadersPolicy.Remove...)
	}

	var responseHeadersPolicy dag.HeadersPolicy
	if ctx.Config.Policy.ResponseHeadersPolicy.Set != nil {
		responseHeadersPolicy.Set = make(map[string]string)
		for k, v := range ctx.Config.Policy.ResponseHeadersPolicy.Set {
			responseHeadersPolicy.Set[k] = v
		}
	}
	if ctx.Config.Policy.ResponseHeadersPolicy.Remove != nil {
		responseHeadersPolicy.Remove = make([]string, 0, len(ctx.Config.Policy.ResponseHeadersPolicy.Remove))
		responseHeadersPolicy.Remove = append(responseHeadersPolicy.Remove, ctx.Config.Policy.ResponseHeadersPolicy.Remove...)
	}

	// Get the appropriate DAG processors.
	dagProcessors := []dag.Processor{
		&dag.IngressProcessor{
			FieldLogger:       log.WithField("context", "IngressProcessor"),
			ClientCertificate: clientCert,
		},
		&dag.ExtensionServiceProcessor{
			FieldLogger:       log.WithField("context", "ExtensionServiceProcessor"),
			ClientCertificate: clientCert,
		},
		&dag.HTTPProxyProcessor{
			DisablePermitInsecure: ctx.Config.DisablePermitInsecure,
			FallbackCertificate:   fallbackCert,
			DNSLookupFamily:       ctx.Config.Cluster.DNSLookupFamily,
			ClientCertificate:     clientCert,
			RequestHeadersPolicy:  &requestHeadersPolicy,
			ResponseHeadersPolicy: &responseHeadersPolicy,
		},
	}

	if ctx.Config.GatewayConfig != nil && clients.ResourcesExist(k8s.GatewayAPIResources()...) {
		dagProcessors = append(dagProcessors, &dag.GatewayAPIProcessor{
			FieldLogger: log.WithField("context", "GatewayAPIProcessor"),
		})
	}

	// The listener processor has to go last since it looks at
	// the output of the other processors.
	dagProcessors = append(dagProcessors, &dag.ListenerProcessor{})

	var configuredSecretRefs []*types.NamespacedName
	if fallbackCert != nil {
		configuredSecretRefs = append(configuredSecretRefs, fallbackCert)
	}
	if clientCert != nil {
		configuredSecretRefs = append(configuredSecretRefs, clientCert)
	}

	builder := dag.Builder{
		Source: dag.KubernetesCache{
			RootNamespaces:       ctx.proxyRootNamespaces(),
			IngressClassName:     ctx.ingressClassName,
			ConfiguredSecretRefs: configuredSecretRefs,
			FieldLogger:          log.WithField("context", "KubernetesCache"),
		},
		Processors: dagProcessors,
	}

	// govet complains about copying the sync.Once that's in the dag.KubernetesCache
	// but it's safe to ignore since this function is only called once.
	// nolint:govet
	return builder
}

func contains(namespaces []string, ns string) bool {
	for _, namespace := range namespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

func informOnResource(clients *k8s.Clients, gvr schema.GroupVersionResource, handler cache.ResourceEventHandler) error {
	inf, err := clients.InformerForResource(gvr)
	if err != nil {
		return err
	}

	inf.AddEventHandler(handler)
	return nil
}
