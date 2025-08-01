package routing

import (
	"net/http"

	"golang.org/x/exp/maps"

	"github.com/kiali/kiali/business"
	"github.com/kiali/kiali/cache"
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/grafana"
	"github.com/kiali/kiali/handlers"
	"github.com/kiali/kiali/handlers/authentication"
	"github.com/kiali/kiali/istio"
	"github.com/kiali/kiali/kubernetes"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/prometheus"
	"github.com/kiali/kiali/tracing"
)

// Route describes a single route
type Route struct {
	Name          string
	LogGroupName  string
	Method        string
	Pattern       string
	HandlerFunc   http.HandlerFunc
	Authenticated bool
}

// Routes holds an array of Route. A note on swagger documentation. The path variables and query parameters
// are defined in ../doc.go.  YOu need to manually associate params and routes.
type Routes struct {
	Routes []Route
}

// NewRoutes creates and returns all the API routes
func NewRoutes(
	conf *config.Config,
	kialiCache cache.KialiCache,
	clientFactory kubernetes.ClientFactory,
	cpm business.ControlPlaneMonitor,
	prom prometheus.ClientInterface,
	traceClientLoader func() tracing.ClientInterface,
	authController authentication.AuthController,
	grafana *grafana.Service,
	discovery *istio.Discovery,
) (r *Routes) {
	r = new(Routes)

	r.Routes = []Route{
		// swagger:route GET /healthz kiali healthz
		// ---
		// Endpoint to get the health of Kiali
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		// responses:
		//		500: internalError
		//		200
		{
			"Healthz",
			log.StatusLogName,
			"GET",
			"/healthz",
			handlers.Healthz,
			false,
		},
		// swagger:route GET / kiali root
		// ---
		// Endpoint to get the status of Kiali
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		// responses:
		//      500: internalError
		//      200: statusInfo
		{
			"Root",
			log.StatusLogName,
			"GET",
			"/api",
			handlers.Root(conf, clientFactory, kialiCache, grafana),
			conf.Server.RequireAuth,
		},
		// swagger:route GET /authenticate auth authenticate
		// ---
		// Endpoint to authenticate the user
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		//    Security:
		//     authorization: user, password
		//
		// responses:
		//      500: internalError
		//      200: userSessionData
		{
			"Authenticate",
			log.AuthenticateLogName,
			"GET",
			"/api/authenticate",
			handlers.Authenticate(conf, authController),
			false,
		},
		// swagger:route POST /authenticate auth openshiftCheckToken
		// ---
		// Endpoint to check if a token from Openshift is working correctly
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: userSessionData
		{
			"OpenshiftCheckToken",
			log.AuthenticateLogName,
			"POST",
			"/api/authenticate",
			handlers.Authenticate(conf, authController),
			false,
		},
		// swagger:route GET /logout auth logout
		// ---
		// Endpoint to logout an user (unset the session cookie)
		//
		//     Schemes: http, https
		//
		// responses:
		//      204: noContent
		{
			"Logout",
			log.AuthenticateLogName,
			"GET",
			"/api/logout",
			handlers.Logout(conf, authController),
			false,
		},
		// swagger:route GET /auth/info auth authenticationInfo
		// ---
		// Endpoint to get login info, such as strategy, authorization endpoints
		// for OAuth providers and so on.
		//
		//     Consumes:
		//     - application/json
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: authenticationInfo
		{
			"AuthenticationInfo",
			log.AuthenticateLogName,
			"GET",
			"/api/auth/info",
			handlers.AuthenticationInfo(conf, authController, maps.Keys(clientFactory.GetSAClients())),
			false,
		},
		// swagger:route GET /status status getStatus
		// ---
		// Endpoint to get the status of Kiali
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: statusInfo
		{
			"Status",
			log.StatusLogName,
			"GET",
			"/api/status",
			handlers.Root(conf, clientFactory, kialiCache, grafana),
			true,
		},
		// swagger:route GET /tracing/diagnose tracing tracingDiagnose
		// ---
		// Endpoint to get a diagnose for the tracing endpoint
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: tracingDiagnose
		{
			"Diagnose",
			log.TracingLogName,
			"GET",
			"/api/tracing/diagnose",
			handlers.TracingDiagnose(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /tracing/check tracing check config
		// ---
		// Endpoint to test a specific configuration for tracing
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: configurationValidation
		{
			"Tracing check",
			log.TracingLogName,
			"POST",
			"/api/tracing/check",
			handlers.TracingConfigurationCheck(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /config kiali getConfig
		// ---
		// Endpoint to get the config of Kiali
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: statusInfo
		{
			"Config",
			log.ConfigLogName,
			"GET",
			"/api/config",
			handlers.Config(conf, kialiCache, discovery, clientFactory, prom),
			true,
		},
		// swagger:route GET /crippled kiali getCrippledFeatures
		// ---
		// Endpoint to get the crippled features of Kiali
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: statusInfo
		{
			"Crippled",
			log.ConfigLogName,
			"GET",
			"/api/crippled",
			handlers.CrippledFeatures(prom),
			true,
		},
		// swagger:route GET /istio/permissions config getPermissions
		// ---
		// Endpoint to get the caller permissions on new Istio Config objects
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: istioConfigPermissions
		{
			"IstioConfigPermissions",
			log.IstioConfigLogName,
			"GET",
			"/api/istio/permissions",
			handlers.IstioConfigPermissions(conf, kialiCache, clientFactory, prom, traceClientLoader, discovery, cpm, grafana),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/istio config istioConfigList
		// ---
		// Endpoint to get the list of Istio Config of a namespace
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: istioConfigList
		//
		{
			"IstioConfigList",
			log.IstioConfigLogName,
			"GET",
			"/api/namespaces/{namespace}/istio",
			handlers.IstioConfigList(conf, kialiCache, clientFactory, prom, traceClientLoader, discovery, cpm, grafana),
			true,
		},
		// swagger:route GET /istio config istioConfigListAll
		// ---
		// Endpoint to get the list of Istio Config of all namespaces
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: istioConfigList
		//
		{
			"IstioConfigListAll",
			log.IstioConfigLogName,
			"GET",
			"/api/istio/config",
			handlers.IstioConfigList(conf, kialiCache, clientFactory, prom, traceClientLoader, discovery, cpm, grafana),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/istio/{group}/{version}/{kind}/{object} config istioConfigDetails
		// ---
		// Endpoint to get the Istio Config of an Istio object
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      404: notFoundError
		//      500: internalError
		//      200: istioConfigDetailsResponse
		//
		{
			"IstioConfigDetails",
			log.IstioConfigLogName,
			"GET",
			"/api/namespaces/{namespace}/istio/{group}/{version}/{kind}/{object}",
			handlers.IstioConfigDetails(conf, kialiCache, clientFactory, prom, traceClientLoader, discovery, cpm, grafana),
			true,
		},
		// swagger:route DELETE /namespaces/{namespace}/istio/{group}/{version}/{kind}/{object} config istioConfigDelete
		// ---
		// Endpoint to delete the Istio Config of an (arbitrary) Istio object
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      404: notFoundError
		//      500: internalError
		//      200
		//
		{
			"IstioConfigDelete",
			log.IstioConfigLogName,
			"DELETE",
			"/api/namespaces/{namespace}/istio/{group}/{version}/{kind}/{object}",
			handlers.IstioConfigDelete(conf, kialiCache, clientFactory, prom, traceClientLoader, discovery, cpm, grafana),
			true,
		},
		// swagger:route PATCH /namespaces/{namespace}/istio/{group}/{version}/{kind}/{object} config istioConfigUpdate
		// ---
		// Endpoint to update the Istio Config of an Istio object used for templates and adapters using Json Merge Patch strategy.
		//
		//     Consumes:
		//	   - application/json
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      404: notFoundError
		//      500: internalError
		//      200: istioConfigDetailsResponse
		//
		{
			"IstioConfigUpdate",
			log.IstioConfigLogName,
			"PATCH",
			"/api/namespaces/{namespace}/istio/{group}/{version}/{kind}/{object}",
			handlers.IstioConfigUpdate(conf, kialiCache, clientFactory, prom, traceClientLoader, discovery, cpm, grafana),
			true,
		},
		// swagger:route POST /namespaces/{namespace}/istio/{group}/{version}/{kind} config istioConfigCreate
		// ---
		// Endpoint to create an Istio object by using an Istio Config item
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      404: notFoundError
		//      500: internalError
		//		202
		//		201: istioConfigDetailsResponse
		//      200: istioConfigDetailsResponse
		//
		{
			"IstioConfigCreate",
			log.IstioConfigLogName,
			"POST",
			"/api/namespaces/{namespace}/istio/{group}/{version}/{kind}",
			handlers.IstioConfigCreate(conf, kialiCache, clientFactory, prom, traceClientLoader, discovery, cpm, grafana),
			true,
		},
		// swagger:route GET /clusters/services services serviceList
		// ---
		// Endpoint to get the list of services for a given cluster
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: serviceListResponse
		//
		{
			"ClustersServices",
			log.ClustersLogName,
			"GET",
			"/api/clusters/services",
			handlers.ClustersServices(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/services/{service} services serviceDetails
		// ---
		// Endpoint to get the details of a given service
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      404: notFoundError
		//      500: internalError
		//      200: serviceDetailsResponse
		//
		{
			"ServiceDetails",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces/{namespace}/services/{service}",
			handlers.ServiceDetails(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route PATCH /namespaces/{namespace}/services/{service} services serviceUpdate
		// ---
		// Endpoint to update the Service configuration using Json Merge Patch strategy.
		//
		//     Consumes:
		//	   - application/json
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      404: notFoundError
		//      500: internalError
		//      200: serviceDetailsResponse
		//
		{
			"ServiceUpdate",
			log.ResourcesLogName,
			"PATCH",
			"/api/namespaces/{namespace}/services/{service}",
			handlers.ServiceUpdate(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/apps/{app}/spans traces appSpans
		// ---
		// Endpoint to get Tracing spans for a given app
		//
		//		Produces:
		//		- application/json
		//
		//		Schemes: http, https
		//
		// responses:
		// 		500: internalError
		//		200: spansResponse
		{
			"AppSpans",
			log.TracingLogName,
			"GET",
			"/api/namespaces/{namespace}/apps/{app}/spans",
			handlers.AppSpans(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/workloads/{workload}/spans traces workloadSpans
		// ---
		// Endpoint to get Tracing spans for a given workload
		//
		//		Produces:
		//		- application/json
		//
		//		Schemes: http, https
		//
		// responses:
		// 		500: internalError
		//		200: spansResponse
		{
			"WorkloadSpans",
			log.TracingLogName,
			"GET",
			"/api/namespaces/{namespace}/workloads/{workload}/spans",
			handlers.WorkloadSpans(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/services/{service}/spans traces serviceSpans
		// ---
		// Endpoint to get Tracing spans for a given service
		//
		//		Produces:
		//		- application/json
		//
		//		Schemes: http, https
		//
		// responses:
		// 		500: internalError
		//		200: spansResponse
		{
			"ServiceSpans",
			log.TracingLogName,
			"GET",
			"/api/namespaces/{namespace}/services/{service}/spans",
			handlers.ServiceSpans(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/apps/{app}/traces traces appTraces
		// ---
		// Endpoint to get the traces of a given app
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      404: notFoundError
		//      500: internalError
		//      200: traceDetailsResponse
		//
		{
			"AppTraces",
			log.TracingLogName,
			"GET",
			"/api/namespaces/{namespace}/apps/{app}/traces",
			handlers.AppTraces(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/services/{service}/traces traces serviceTraces
		// ---
		// Endpoint to get the traces of a given service
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      404: notFoundError
		//      500: internalError
		//      200: traceDetailsResponse
		//
		{
			"ServiceTraces",
			log.TracingLogName,
			"GET",
			"/api/namespaces/{namespace}/services/{service}/traces",
			handlers.ServiceTraces(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/workloads/{workload}/traces traces workloadTraces
		// ---
		// Endpoint to get the traces of a given workload
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      404: notFoundError
		//      500: internalError
		//      200: traceDetailsResponse
		//
		{
			"WorkloadTraces",
			log.TracingLogName,
			"GET",
			"/api/namespaces/{namespace}/workloads/{workload}/traces",
			handlers.WorkloadTraces(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/apps/{app}/errortraces traces errorTraces
		// ---
		// Endpoint to get the number of traces in error for a given service
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      404: notFoundError
		//      500: internalError
		//      200: errorTracesResponse
		//
		{
			"ErrorTraces",
			log.TracingLogName,
			"GET",
			"/api/namespaces/{namespace}/apps/{app}/errortraces",
			handlers.ErrorTraces(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /traces/{traceID} traces traceDetails
		// ---
		// Endpoint to get a specific trace from ID
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      404: notFoundError
		//      500: internalError
		//      200: traceDetailsResponse
		//
		{
			"TracesDetails",
			log.TracingLogName,
			"GET",
			"/api/traces/{traceID}",
			handlers.TraceDetails(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /clusters/workloads workloads workloadList
		// ---
		// Endpoint to get the list of workloads for a cluster
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: workloadListResponse
		//
		{
			"ClusterWorkloads",
			log.ClustersLogName,
			"GET",
			"/api/clusters/workloads",
			handlers.ClusterWorkloads(conf, kialiCache, clientFactory, cpm, prom, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/workloads/{workload} workloads workloadDetails
		// ---
		// Endpoint to get the workload details
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      404: notFoundError
		//      200: workloadDetails
		//
		{
			"WorkloadDetails",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces/{namespace}/workloads/{workload}",
			handlers.WorkloadDetails(conf, kialiCache, clientFactory, cpm, prom, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route PATCH /namespaces/{namespace}/workloads/{workload} workloads workloadUpdate
		// ---
		// Endpoint to update the Workload configuration using Json Merge Patch strategy.
		//
		//     Consumes:
		//	   - application/json
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      404: notFoundError
		//      500: internalError
		//      200: workloadDetails
		//
		{
			"WorkloadUpdate",
			log.ResourcesLogName,
			"PATCH",
			"/api/namespaces/{namespace}/workloads/{workload}",
			handlers.WorkloadUpdate(conf, kialiCache, clientFactory, cpm, prom, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /clusters/apps apps appList
		// ---
		// Endpoint to get the list of apps for a cluster
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: appListResponse
		//
		{
			"ClustersApps",
			log.ClustersLogName,
			"GET",
			"/api/clusters/apps",
			handlers.ClusterApps(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/apps/{app} apps appDetails
		// ---
		// Endpoint to get the app details
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      404: notFoundError
		//      200: appDetails
		//
		{
			"AppDetails",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces/{namespace}/apps/{app}",
			handlers.AppDetails(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces namespaces namespaceList
		// ---
		// Endpoint to get the list of the available namespaces
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: namespaceList
		//
		{
			"NamespaceList",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces",
			handlers.NamespaceList(conf, kialiCache, clientFactory, discovery),
			true,
		},
		// swagger:route PATCH /namespaces/{namespace} namespaces namespaceUpdate
		// ---
		// Endpoint to update the Namespace configuration using Json Merge Patch strategy.
		//
		//     Consumes:
		//	   - application/json
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      404: notFoundError
		//      500: internalError
		//      200: namespaceResponse
		//
		{
			"NamespaceUpdate",
			log.ResourcesLogName,
			"PATCH",
			"/api/namespaces/{namespace}",
			handlers.NamespaceUpdate(conf, kialiCache, clientFactory, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/info namespaces namespaceInfo
		// ---
		// Endpoint to get info about a single namespace
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      200: namespaceList
		//
		{
			"NamespaceInfo",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces/{namespace}/info",
			handlers.NamespaceInfo(conf, kialiCache, clientFactory, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/services/{service}/metrics services serviceMetrics
		// ---
		// Endpoint to fetch metrics to be displayed, related to a single service
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: metricsResponse
		//
		{
			"ServiceMetrics",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/services/{service}/metrics",
			handlers.ServiceMetrics(conf, kialiCache, discovery, clientFactory, prom),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/aggregates/{aggregate}/{aggregateValue}/metrics aggregates aggregateMetrics
		// ---
		// Endpoint to fetch metrics to be displayed, related to a single aggregate
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: metricsResponse
		//
		{
			"AggregateMetrics",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/aggregates/{aggregate}/{aggregateValue}/metrics",
			handlers.AggregateMetrics(conf, kialiCache, discovery, clientFactory, prom),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/apps/{app}/metrics apps appMetrics
		// ---
		// Endpoint to fetch metrics to be displayed, related to a single app
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: metricsResponse
		//
		{
			"AppMetrics",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/apps/{app}/metrics",
			handlers.AppMetrics(conf, kialiCache, discovery, clientFactory, prom),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/workloads/{workload}/metrics workloads workloadMetrics
		// ---
		// Endpoint to fetch metrics to be displayed, related to a single workload
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: metricsResponse
		//
		{
			"WorkloadMetrics",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/workloads/{workload}/metrics",
			handlers.WorkloadMetrics(conf, kialiCache, discovery, clientFactory, prom),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/controlplanes/{controlplane}/metrics controlplanes controlPlaneMetrics
		// ---
		// Endpoint to fetch metrics to be displayed, related to a single control plane
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: metricsResponse
		//
		{
			"ControlPlaneMetrics",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/controlplanes/{controlplane}/metrics",
			handlers.ControlPlaneMetrics(conf, kialiCache, discovery, clientFactory, prom, cpm, traceClientLoader, grafana),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/{app}/usage_metrics resource usageMetrics
		// ---
		// Endpoint to fetch metrics to be displayed, related to cpu and memory usage
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: metricsResponse
		//
		{
			"UsageMetrics",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/{app}/usage_metrics",
			handlers.ResourceUsageMetrics(conf, kialiCache, discovery, clientFactory, prom),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/services/{service}/dashboard services serviceDashboard
		// ---
		// Endpoint to fetch dashboard to be displayed, related to a single service
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: dashboardResponse
		//
		{
			"ServiceDashboard",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/services/{service}/dashboard",
			handlers.ServiceDashboard(conf, kialiCache, clientFactory, cpm, prom, traceClientLoader, discovery, grafana),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/apps/{app}/dashboard apps appDashboard
		// ---
		// Endpoint to fetch dashboard to be displayed, related to a single app
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: dashboardResponse
		//
		{
			"AppDashboard",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/apps/{app}/dashboard",
			handlers.AppDashboard(conf, kialiCache, discovery, clientFactory, prom, grafana),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/workloads/{workload}/dashboard workloads workloadDashboard
		// ---
		// Endpoint to fetch dashboard to be displayed, related to a single workload
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: dashboardResponse
		//
		{
			"WorkloadDashboard",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/workloads/{workload}/dashboard",
			handlers.WorkloadDashboard(conf, kialiCache, clientFactory, discovery, prom, grafana),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/ztunnel/{workload}/dashboard workloads ztunnelDashboard
		// ---
		// Endpoint to fetch dashboard to be displayed, related to a ztunnel workload
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: dashboardResponse
		//
		{
			"ZtunnelDashboard",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/ztunnel/{workload}/dashboard",
			handlers.ZtunnelDashboard(conf, kialiCache, discovery, clientFactory, grafana, prom),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/customdashboard/{dashboard} dashboards customDashboard
		// ---
		// Endpoint to fetch a custom dashboard
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: dashboardResponse
		//
		{
			"CustomDashboard",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/customdashboard/{dashboard}",
			handlers.CustomDashboard(conf, kialiCache, clientFactory, discovery, grafana, prom, traceClientLoader, cpm),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/metrics namespaces namespaceMetrics
		// ---
		// Endpoint to fetch metrics to be displayed, related to a namespace
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: metricsResponse
		//
		{
			"NamespaceMetrics",
			log.MetricsLogName,
			"GET",
			"/api/namespaces/{namespace}/metrics",
			handlers.NamespaceMetrics(conf, kialiCache, discovery, clientFactory, prom),
			true,
		},
		// swagger:route GET /clusters/health cluster namespaces Health
		// ---
		// Get health for all objects in namespaces of the given cluster
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      200: clustersNamespaceHealthResponse
		//      400: badRequestError
		//      500: internalError
		//
		{
			"ClustersHealth",
			log.ClustersLogName,
			"GET",
			"/api/clusters/health",
			handlers.ClusterHealth(conf, kialiCache, clientFactory, prom, traceClientLoader, discovery, cpm, grafana),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/validations namespaces namespaceValidations
		// ---
		// Get validation summary for all objects in the given namespace
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      200: namespaceValidationSummaryResponse
		//      400: badRequestError
		//      500: internalError
		//
		{
			"NamespaceValidationSummary",
			log.ValidationLogName,
			"GET",
			"/api/namespaces/{namespace}/validations",
			handlers.NamespaceValidationSummary(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /istio/validations namespaces namespacesValidations
		// ---
		// Get validation summary for all objects in the given namespaces
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      200: namespaceValidationSummaryResponse
		//      400: badRequestError
		//      500: internalError
		//
		{
			"ConfigValidationSummary",
			log.ValidationLogName,
			"GET",
			"/api/istio/validations",
			handlers.IstioConfigValidationSummary(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /mesh/tls tls meshTls
		// ---
		// Get TLS status for the whole mesh
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      200: meshTlsResponse
		//      400: badRequestError
		//      500: internalError
		//
		{
			"MeshTls",
			log.StatusLogName,
			"GET",
			"/api/mesh/tls",
			handlers.MeshTls(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/tls tls namespaceTls
		// ---
		// Get TLS status for the given namespace
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      200: namespaceTlsResponse
		//      400: badRequestError
		//      500: internalError
		//
		{
			"NamespaceTls",
			log.StatusLogName,
			"GET",
			"/api/namespaces/{namespace}/tls",
			handlers.NamespaceTls(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /clusters/tls tls ClustersTls
		// ---
		// Get TLS statuses for given namespaces of the given cluster
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      200: clusterTlsResponse
		//      400: badRequestError
		//      500: internalError
		//
		{
			"ClusterTls",
			log.ClustersLogName,
			"GET",
			"/api/clusters/tls",
			handlers.ClustersTls(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /istio/status status istioStatus
		// ---
		// Get the status of each components needed in the control plane
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      200: istioStatusResponse
		//      400: badRequestError
		//      500: internalError
		//
		{
			"IstioStatus",
			log.StatusLogName,
			"GET",
			"/api/istio/status",
			handlers.IstioStatus(conf, kialiCache, clientFactory, prom, traceClientLoader, discovery, cpm, grafana),
			true,
		},
		// swagger:route GET /namespaces/graph graphs graphNamespaces
		// ---
		// The backing JSON for a namespaces graph.
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      500: internalError
		//      200: graphResponse
		//
		{
			"GraphNamespaces",
			log.GraphLogName,
			"GET",
			"/api/namespaces/graph",
			handlers.GraphNamespaces(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/aggregates/{aggregate}/{aggregateValue}/graph graphs graphAggregate
		// ---
		// The backing JSON for an aggregate node detail graph. (supported graphTypes: app | versionedApp | workload)
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      500: internalError
		//      200: graphResponse
		//
		{
			"GraphAggregate",
			log.GraphLogName,
			"GET",
			"/api/namespaces/{namespace}/aggregates/{aggregate}/{aggregateValue}/graph",
			handlers.GraphNode(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/aggregates/{aggregate}/{aggregateValue}/{service}/graph graphs graphAggregateByService
		// ---
		// The backing JSON for an aggregate node detail graph, specific to a service. (supported graphTypes: app | versionedApp | workload)
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      500: internalError
		//      200: graphResponse
		//
		{
			"GraphAggregateByService",
			log.GraphLogName,
			"GET",
			"/api/namespaces/{namespace}/aggregates/{aggregate}/{aggregateValue}/{service}/graph",
			handlers.GraphNode(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/applications/{app}/versions/{version}/graph graphs graphAppVersion
		// ---
		// The backing JSON for a versioned app node detail graph. (supported graphTypes: app | versionedApp)
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      500: internalError
		//      200: graphResponse
		//
		{
			"GraphAppVersion",
			log.GraphLogName,
			"GET",
			"/api/namespaces/{namespace}/applications/{app}/versions/{version}/graph",
			handlers.GraphNode(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/applications/{app}/graph graphs graphApp
		// ---
		// The backing JSON for an app node detail graph. (supported graphTypes: app | versionedApp)
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      500: internalError
		//      200: graphResponse
		//
		{
			"GraphApp",
			log.GraphLogName,
			"GET",
			"/api/namespaces/{namespace}/applications/{app}/graph",
			handlers.GraphNode(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/services/{service}/graph graphs graphService
		// ---
		// The backing JSON for a service node detail graph.
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      500: internalError
		//      200: graphResponse
		//
		{
			"GraphService",
			log.GraphLogName,
			"GET",
			"/api/namespaces/{namespace}/services/{service}/graph",
			handlers.GraphNode(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/workloads/{workload}/graph graphs graphWorkload
		// ---
		// The backing JSON for a workload node detail graph.
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      500: internalError
		//      200: graphResponse
		//
		{
			"GraphWorkload",
			log.GraphLogName,
			"GET",
			"/api/namespaces/{namespace}/workloads/{workload}/graph",
			handlers.GraphNode(conf, kialiCache, clientFactory, prom, cpm, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /mesh/graph meshGraph
		// ---
		// The backing JSON for a mesh graph
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      500: internalError
		//      200: graphResponse
		//
		{
			"MeshGraph",
			log.GraphLogName,
			"GET",
			"/api/mesh/graph",
			handlers.MeshGraph(conf, clientFactory, kialiCache, grafana, prom, traceClientLoader, discovery, cpm),
			true,
		},
		// swagger:route GET /mesh/controlplanes controlplanes
		// ---
		// The backing JSON for mesh controlplanes
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      500: internalError
		//      200: graphResponse
		//
		{
			"MeshControlPlanes",
			log.MeshLogName,
			"GET",
			"/api/mesh/controlplanes",
			handlers.ControlPlanes(kialiCache, clientFactory, conf, discovery),
			true,
		},
		// swagger:route GET /grafana integrations grafanaInfo
		// ---
		// Get the grafana URL and other descriptors
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      503: serviceUnavailableError
		//      200: grafanaInfoResponse
		//      204: noContent
		//
		{
			"GrafanaURL",
			log.ConfigLogName,
			"GET",
			"/api/grafana",
			handlers.GetGrafanaInfo(conf, grafana),
			true,
		},
		// swagger:route GET /tracing integrations tracingInfo
		// ---
		// Get the tracing URL and other descriptors
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      404: notFoundError
		//      406: notAcceptableError
		//      200: tracingInfoResponse
		//
		{
			"TracingURL",
			log.ConfigLogName,
			"GET",
			"/api/tracing",
			handlers.GetTracingInfo(conf),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/pods/{pod} pods podDetails
		// ---
		// Endpoint to get pod details
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      404: notFoundError
		//      200: workloadDetails
		//
		{
			"PodDetails",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces/{namespace}/pods/{pod}",
			handlers.PodDetails(conf, kialiCache, clientFactory, cpm, prom, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/pods/{pod}/logs pods podLogs
		// ---
		// Endpoint to get pod logs
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      404: notFoundError
		//      200: workloadDetails
		//
		{
			"PodLogs",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces/{namespace}/pods/{pod}/logs",
			handlers.PodLogs(conf, kialiCache, clientFactory, cpm, prom, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/pods/{pod}/config_dump pods podProxyDump
		// ---
		// Endpoint to get pod proxy dump
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      404: notFoundError
		//      200: configDump
		//
		{
			"PodConfigDump",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces/{namespace}/pods/{pod}/config_dump",
			handlers.ConfigDump(conf, kialiCache, clientFactory, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/pods/{pod}/config_dump/{resource} pods podProxyResource
		// ---
		// Endpoint to get pod resource entries
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      404: notFoundError
		//      200: configDumpResource
		//
		{
			"PodProxyResource",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces/{namespace}/pods/{pod}/config_dump/{resource}",
			handlers.ConfigDumpResourceEntries(conf, kialiCache, clientFactory, discovery),
			true,
		},
		// swagger:route GET /namespaces/{namespace}/pods/{pod}/config_dump_ztunnel
		// ---
		// Endpoint to get ztunnel pod config dump
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      404: notFoundError
		//      200: ztunnelConfigDump
		//
		{
			"ZtunnelConfigDump",
			log.ResourcesLogName,
			"GET",
			"/api/namespaces/{namespace}/pods/{pod}/config_dump_ztunnel",
			handlers.ConfigDumpZtunnel(conf, kialiCache, clientFactory, cpm, prom, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route POST /namespaces/{namespace}/pods/{pod}/logging pods podProxyLogging
		// ---
		// Endpoint to set pod proxy log level
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      500: internalError
		//      404: notFoundError
		//      400: badRequestError
		//      200: noContent
		//
		{
			"PodProxyLogging",
			log.ResourcesLogName,
			"POST",
			"/api/namespaces/{namespace}/pods/{pod}/logging",
			handlers.LoggingUpdate(conf, kialiCache, clientFactory, cpm, prom, traceClientLoader, grafana, discovery),
			true,
		},
		// swagger:route GET /clusters/metrics clusterName namespaces clustersMetrics
		// ---
		// Endpoint to fetch metrics to be displayed, related to all provided namespaces of provided cluster
		//
		//     Produces:
		//     - application/json
		//
		//     Schemes: http, https
		//
		// responses:
		//      400: badRequestError
		//      503: serviceUnavailableError
		//      200: metricsResponse
		//
		{
			"ClustersMetrics",
			log.ClustersLogName,
			"GET",
			"/api/clusters/metrics",
			handlers.ClustersMetrics(conf, kialiCache, discovery, clientFactory, prom),
			true,
		},
		// swagger:route POST /stats/metrics stats metricsStats
		// ---
		// Produces metrics statistics
		//
		// 		Produces:
		//		- application/json
		//
		//		Schemes: http, https
		//
		// responses:
		//    400: badRequestError
		//    503: serviceUnavailableError
		//		500: internalError
		//		200: metricsStatsResponse
		{
			Name:          "MetricsStats",
			LogGroupName:  log.MetricsLogName,
			Method:        "POST",
			Pattern:       "/api/stats/metrics",
			HandlerFunc:   handlers.MetricsStats(conf, kialiCache, discovery, clientFactory, prom),
			Authenticated: true,
		},
	}
	return
}
