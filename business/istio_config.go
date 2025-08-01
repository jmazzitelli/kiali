package business

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	extentions_v1alpha1 "istio.io/client-go/pkg/apis/extensions/v1alpha1"
	networking_v1 "istio.io/client-go/pkg/apis/networking/v1"
	networking_v1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	security_v1 "istio.io/client-go/pkg/apis/security/v1"
	telemetry_v1 "istio.io/client-go/pkg/apis/telemetry/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	api_types "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8s_inference_v1alpha2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	k8s_networking_v1 "sigs.k8s.io/gateway-api/apis/v1"
	k8s_networking_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	k8s_networking_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/kiali/kiali/cache"
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/kubernetes"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/models"
	"github.com/kiali/kiali/observability"
	"github.com/kiali/kiali/util"
)

const allResources string = "*"

type IstioConfigService struct {
	userClients         map[string]kubernetes.UserClientInterface
	saClients           map[string]kubernetes.ClientInterface
	conf                *config.Config
	kialiCache          cache.KialiCache
	businessLayer       *Layer
	controlPlaneMonitor ControlPlaneMonitor
}

type IstioConfigCriteria struct {
	IncludeGateways               bool
	IncludeK8sGateways            bool
	IncludeK8sGRPCRoutes          bool
	IncludeK8sHTTPRoutes          bool
	IncludeK8sInferencePools      bool
	IncludeK8sReferenceGrants     bool
	IncludeK8sTCPRoutes           bool
	IncludeK8sTLSRoutes           bool
	IncludeVirtualServices        bool
	IncludeDestinationRules       bool
	IncludeServiceEntries         bool
	IncludeSidecars               bool
	IncludeAuthorizationPolicies  bool
	IncludePeerAuthentications    bool
	IncludeWorkloadEntries        bool
	IncludeWorkloadGroups         bool
	IncludeRequestAuthentications bool
	IncludeEnvoyFilters           bool
	IncludeWasmPlugins            bool
	IncludeTelemetry              bool
	LabelSelector                 string
	WorkloadSelector              string
}

func (icc IstioConfigCriteria) Include(resource schema.GroupVersionKind) bool {
	// Flag used to skip object that are not used in a query when a WorkloadSelector is present
	isWorkloadSelector := icc.WorkloadSelector != ""
	switch resource {
	case kubernetes.Gateways:
		return icc.IncludeGateways
	case kubernetes.K8sGateways:
		return icc.IncludeK8sGateways
	case kubernetes.K8sGRPCRoutes:
		return icc.IncludeK8sGRPCRoutes
	case kubernetes.K8sHTTPRoutes:
		return icc.IncludeK8sHTTPRoutes
	case kubernetes.K8sInferencePools:
		return icc.IncludeK8sInferencePools
	case kubernetes.K8sReferenceGrants:
		return icc.IncludeK8sReferenceGrants
	case kubernetes.K8sTCPRoutes:
		return icc.IncludeK8sTCPRoutes
	case kubernetes.K8sTLSRoutes:
		return icc.IncludeK8sTLSRoutes
	case kubernetes.VirtualServices:
		return icc.IncludeVirtualServices && !isWorkloadSelector
	case kubernetes.DestinationRules:
		return icc.IncludeDestinationRules && !isWorkloadSelector
	case kubernetes.ServiceEntries:
		return icc.IncludeServiceEntries && !isWorkloadSelector
	case kubernetes.Sidecars:
		return icc.IncludeSidecars
	case kubernetes.AuthorizationPolicies:
		return icc.IncludeAuthorizationPolicies
	case kubernetes.PeerAuthentications:
		return icc.IncludePeerAuthentications
	case kubernetes.WorkloadEntries:
		return icc.IncludeWorkloadEntries && !isWorkloadSelector
	case kubernetes.WorkloadGroups:
		return icc.IncludeWorkloadGroups
	case kubernetes.RequestAuthentications:
		return icc.IncludeRequestAuthentications
	case kubernetes.EnvoyFilters:
		return icc.IncludeEnvoyFilters
	case kubernetes.WasmPlugins:
		return icc.IncludeWasmPlugins
	case kubernetes.Telemetries:
		return icc.IncludeTelemetry
	}
	return false
}

// IstioConfig types used in the IstioConfig New Page Form
// networking.istio.io
var newNetworkingConfigTypes = []schema.GroupVersionKind{
	kubernetes.Sidecars,
	kubernetes.Gateways,
	kubernetes.ServiceEntries,
}

// gateway.networking.k8s.io
var newK8sNetworkingConfigTypes = []schema.GroupVersionKind{
	kubernetes.K8sGateways,
	kubernetes.K8sReferenceGrants,
}

// security.istio.io
var newSecurityConfigTypes = []schema.GroupVersionKind{
	kubernetes.AuthorizationPolicies,
	kubernetes.PeerAuthentications,
	kubernetes.RequestAuthentications,
}

// GetIstioConfigMap returns a map of Istio config objects list per cluster
// @TODO this method should replace GetIstioConfigList
func (in *IstioConfigService) GetIstioConfigMap(ctx context.Context, namespace string, criteria IstioConfigCriteria) (models.IstioConfigMap, error) {
	istioConfigMap := models.IstioConfigMap{}
	for cluster := range in.userClients {
		var (
			singleClusterConfigList *models.IstioConfigList
			err                     error
		)
		if namespace == meta_v1.NamespaceAll {
			singleClusterConfigList, err = in.GetIstioConfigList(ctx, cluster, criteria)
			if err != nil {
				return nil, err
			}
		} else {
			singleClusterConfigList, err = in.GetIstioConfigListForNamespace(ctx, cluster, namespace, criteria)
			if err != nil {
				return nil, err
			}
		}

		istioConfigMap[cluster] = *singleClusterConfigList
	}

	return istioConfigMap, nil
}

// GetIstioConfigMap returns a map of Istio config objects list per cluster
// @TODO this method should replace GetIstioConfigList
func (in *IstioConfigService) GetIstioConfigListForCluster(ctx context.Context, cluster, namespace string, criteria IstioConfigCriteria) (*models.IstioConfigList, error) {
	if namespace == meta_v1.NamespaceAll {
		return in.GetIstioConfigList(ctx, cluster, criteria)
	}

	return in.GetIstioConfigListForNamespace(ctx, cluster, namespace, criteria)
}

func (in *IstioConfigService) GetIstioConfigListForNamespace(ctx context.Context, cluster, namespace string, criteria IstioConfigCriteria) (*models.IstioConfigList, error) {
	// Check if user has access to the namespace (RBAC) in cache scenarios and/or
	// if namespace is accessible from Kiali (Deployment.AccessibleNamespaces)
	if _, err := in.businessLayer.Namespace.GetClusterNamespace(ctx, namespace, cluster); err != nil {
		// Check if the namespace exists on the cluster in multi-cluster mode.
		// TODO: Remove this once other business methods stop looping over all clusters.
		if (api_errors.IsNotFound(err) || api_errors.IsForbidden(err)) && len(in.userClients) > 1 {
			return &models.IstioConfigList{}, nil
		}
		return nil, err
	}

	istioConfigs, err := in.getIstioConfigList(ctx, cluster, namespace, criteria)
	if err != nil {
		return nil, err
	}

	return istioConfigs, nil
}

func ToPtrs[T any](s []T) []*T {
	var ret []*T
	for i := range s {
		ret = append(ret, util.AsPtr(s[i]))
	}
	return ret
}

func (in *IstioConfigService) getIstioConfigList(ctx context.Context, cluster string, namespace string, criteria IstioConfigCriteria) (*models.IstioConfigList, error) {
	istioConfigList := &models.IstioConfigList{
		DestinationRules: []*networking_v1.DestinationRule{},
		EnvoyFilters:     []*networking_v1alpha3.EnvoyFilter{},
		Gateways:         []*networking_v1.Gateway{},
		VirtualServices:  []*networking_v1.VirtualService{},
		ServiceEntries:   []*networking_v1.ServiceEntry{},
		Sidecars:         []*networking_v1.Sidecar{},
		WorkloadEntries:  []*networking_v1.WorkloadEntry{},
		WorkloadGroups:   []*networking_v1.WorkloadGroup{},
		WasmPlugins:      []*extentions_v1alpha1.WasmPlugin{},
		Telemetries:      []*telemetry_v1.Telemetry{},

		K8sGateways:        []*k8s_networking_v1.Gateway{},
		K8sGRPCRoutes:      []*k8s_networking_v1.GRPCRoute{},
		K8sHTTPRoutes:      []*k8s_networking_v1.HTTPRoute{},
		K8sInferencePools:  []*k8s_inference_v1alpha2.InferencePool{},
		K8sReferenceGrants: []*k8s_networking_v1beta1.ReferenceGrant{},
		K8sTCPRoutes:       []*k8s_networking_v1alpha2.TCPRoute{},
		K8sTLSRoutes:       []*k8s_networking_v1alpha2.TLSRoute{},

		AuthorizationPolicies:  []*security_v1.AuthorizationPolicy{},
		PeerAuthentications:    []*security_v1.PeerAuthentication{},
		RequestAuthentications: []*security_v1.RequestAuthentication{},
	}

	// This is called from many places but should be ignored for an external kiali home cluster. It's easier to check here
	// than at all of the callers. If not applicable, just return.
	if in.conf.Clustering.IgnoreHomeCluster && cluster == in.conf.KubernetesConfig.ClusterName {
		return istioConfigList, nil
	}

	var end observability.EndFunc
	_, end = observability.StartSpan(ctx, "GetIstioConfigListForNamespace",
		observability.Attribute("package", "business"),
	)
	defer end()

	saClient := in.saClients[cluster]
	if saClient == nil {
		return nil, fmt.Errorf("kiali ServiceAccount Client for cluster [%s] is not found or is not accessible for Kiali", cluster)
	}

	if !saClient.IsIstioAPI() {
		log.FromContext(ctx).Info().Msgf("Cluster [%s] does not have Istio API installed", cluster)
		// Return empty object here since there are no istio config objects in the cluster
		// and returning nil would cause nil pointer dereference in the caller.
		return istioConfigList, nil
	}

	kubeCache, err := in.kialiCache.GetKubeCache(cluster)
	if err != nil {
		return nil, fmt.Errorf("K8s Cache [%s] is not found or is not accessible for Kiali", cluster)
	}

	userClient := in.userClients[cluster]
	if userClient == nil {
		return nil, fmt.Errorf("K8s Client [%s] is not found or is not accessible for Kiali", cluster)
	}

	isWorkloadSelector := criteria.WorkloadSelector != ""
	workloadSelector := ""
	if isWorkloadSelector {
		workloadSelector = criteria.WorkloadSelector
	}

	selector, err := labels.ConvertSelectorToLabelsMap(criteria.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("bad selector: %s", err)
	}
	listOpts := []client.ListOption{client.MatchingLabels(selector), client.InNamespace(namespace)}

	if criteria.Include(kubernetes.DestinationRules) {
		list := &networking_v1.DestinationRuleList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.DestinationRules = list.Items
	}

	if criteria.Include(kubernetes.EnvoyFilters) {
		list := &networking_v1alpha3.EnvoyFilterList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.EnvoyFilters = list.Items
		if isWorkloadSelector {
			istioConfigList.EnvoyFilters = kubernetes.FilterEnvoyFiltersBySelector(workloadSelector, istioConfigList.EnvoyFilters)
		}
	}

	if criteria.Include(kubernetes.Gateways) {
		list := &networking_v1.GatewayList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.Gateways = list.Items

		if isWorkloadSelector {
			istioConfigList.Gateways = kubernetes.FilterGatewaysBySelector(workloadSelector, istioConfigList.Gateways)
		}
	}

	if userClient.IsGatewayAPI() && criteria.Include(kubernetes.K8sGateways) {
		list := &k8s_networking_v1.GatewayList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}

		istioConfigList.K8sGateways = ToPtrs(list.Items)
	}

	if userClient.IsGatewayAPI() && criteria.Include(kubernetes.K8sGRPCRoutes) {
		list := &k8s_networking_v1.GRPCRouteList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.K8sGRPCRoutes = ToPtrs(list.Items)
	}

	if userClient.IsGatewayAPI() && criteria.Include(kubernetes.K8sHTTPRoutes) {
		list := &k8s_networking_v1.HTTPRouteList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.K8sHTTPRoutes = ToPtrs(list.Items)
	}

	if userClient.IsInferenceAPI() && criteria.Include(kubernetes.K8sInferencePools) {
		list := &k8s_inference_v1alpha2.InferencePoolList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.K8sInferencePools = ToPtrs(list.Items)
		if isWorkloadSelector {
			istioConfigList.K8sInferencePools = kubernetes.FilterK8sInferencePoolsBySelector(workloadSelector, istioConfigList.K8sInferencePools)
		}
	}

	if userClient.IsGatewayAPI() && criteria.Include(kubernetes.K8sReferenceGrants) {
		list := &k8s_networking_v1beta1.ReferenceGrantList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.K8sReferenceGrants = ToPtrs(list.Items)
	}

	if userClient.IsExpGatewayAPI() && criteria.Include(kubernetes.K8sTCPRoutes) {
		list := &k8s_networking_v1alpha2.TCPRouteList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.K8sTCPRoutes = ToPtrs(list.Items)
	}

	if userClient.IsExpGatewayAPI() && criteria.Include(kubernetes.K8sTLSRoutes) {
		list := &k8s_networking_v1alpha2.TLSRouteList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.K8sTLSRoutes = ToPtrs(list.Items)
	}

	if criteria.Include(kubernetes.ServiceEntries) {
		list := &networking_v1.ServiceEntryList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.ServiceEntries = list.Items
	}

	if criteria.Include(kubernetes.Sidecars) {
		list := &networking_v1.SidecarList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.Sidecars = list.Items

		if isWorkloadSelector {
			istioConfigList.Sidecars = kubernetes.FilterSidecarsBySelector(workloadSelector, istioConfigList.Sidecars)
		}
	}

	if criteria.Include(kubernetes.VirtualServices) {
		list := &networking_v1.VirtualServiceList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.VirtualServices = list.Items
	}

	if criteria.Include(kubernetes.WorkloadEntries) {
		list := &networking_v1.WorkloadEntryList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.WorkloadEntries = list.Items
	}

	if criteria.Include(kubernetes.WorkloadGroups) {
		list := &networking_v1.WorkloadGroupList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.WorkloadGroups = list.Items

		if isWorkloadSelector {
			istioConfigList.WorkloadGroups = kubernetes.FilterWorkloadGroupsBySelector(workloadSelector, istioConfigList.WorkloadGroups)
		}
	}

	if criteria.Include(kubernetes.WasmPlugins) {
		list := &extentions_v1alpha1.WasmPluginList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.WasmPlugins = list.Items
	}

	if criteria.Include(kubernetes.Telemetries) {
		list := &telemetry_v1.TelemetryList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.Telemetries = list.Items
	}

	if criteria.Include(kubernetes.AuthorizationPolicies) {
		list := &security_v1.AuthorizationPolicyList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.AuthorizationPolicies = list.Items

		if isWorkloadSelector {
			istioConfigList.AuthorizationPolicies = kubernetes.FilterAuthorizationPoliciesBySelector(workloadSelector, istioConfigList.AuthorizationPolicies)
		}
	}

	if criteria.Include(kubernetes.PeerAuthentications) {
		list := &security_v1.PeerAuthenticationList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.PeerAuthentications = list.Items

		if isWorkloadSelector {
			istioConfigList.PeerAuthentications = kubernetes.FilterPeerAuthenticationsBySelector(workloadSelector, istioConfigList.PeerAuthentications)
		}
	}

	if criteria.Include(kubernetes.RequestAuthentications) {
		list := &security_v1.RequestAuthenticationList{}
		if err := kubeCache.List(ctx, list, listOpts...); err != nil {
			return nil, err
		}
		istioConfigList.RequestAuthentications = list.Items

		if isWorkloadSelector {
			istioConfigList.RequestAuthentications = kubernetes.FilterRequestAuthenticationsBySelector(workloadSelector, istioConfigList.RequestAuthentications)
		}
	}

	return istioConfigList, nil
}

func (in *IstioConfigService) GetIstioConfigList(ctx context.Context, cluster string, criteria IstioConfigCriteria) (*models.IstioConfigList, error) {
	var end observability.EndFunc
	ctx, end = observability.StartSpan(ctx, "GetIstioConfigList",
		observability.Attribute("package", "business"),
	)
	defer end()

	istioConfigs, err := in.getIstioConfigList(ctx, cluster, meta_v1.NamespaceAll, criteria)
	if err != nil {
		return nil, err
	}

	// Filter out namespaces that the user doesn't have access to.
	namespaces, err := in.businessLayer.Namespace.GetClusterNamespaces(ctx, cluster)
	if err != nil {
		return nil, err
	}

	namespaceNames := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		namespaceNames = append(namespaceNames, namespace.Name)
	}

	return &models.IstioConfigList{
		AuthorizationPolicies:  kubernetes.FilterByNamespaceNames(istioConfigs.AuthorizationPolicies, namespaceNames),
		DestinationRules:       kubernetes.FilterByNamespaceNames(istioConfigs.DestinationRules, namespaceNames),
		EnvoyFilters:           kubernetes.FilterByNamespaceNames(istioConfigs.EnvoyFilters, namespaceNames),
		Gateways:               kubernetes.FilterByNamespaceNames(istioConfigs.Gateways, namespaceNames),
		K8sGateways:            kubernetes.FilterByNamespaceNames(istioConfigs.K8sGateways, namespaceNames),
		K8sGRPCRoutes:          kubernetes.FilterByNamespaceNames(istioConfigs.K8sGRPCRoutes, namespaceNames),
		K8sHTTPRoutes:          kubernetes.FilterByNamespaceNames(istioConfigs.K8sHTTPRoutes, namespaceNames),
		K8sInferencePools:      kubernetes.FilterByNamespaceNames(istioConfigs.K8sInferencePools, namespaceNames),
		K8sReferenceGrants:     kubernetes.FilterByNamespaceNames(istioConfigs.K8sReferenceGrants, namespaceNames),
		K8sTCPRoutes:           kubernetes.FilterByNamespaceNames(istioConfigs.K8sTCPRoutes, namespaceNames),
		K8sTLSRoutes:           kubernetes.FilterByNamespaceNames(istioConfigs.K8sTLSRoutes, namespaceNames),
		PeerAuthentications:    kubernetes.FilterByNamespaceNames(istioConfigs.PeerAuthentications, namespaceNames),
		RequestAuthentications: kubernetes.FilterByNamespaceNames(istioConfigs.RequestAuthentications, namespaceNames),
		ServiceEntries:         kubernetes.FilterByNamespaceNames(istioConfigs.ServiceEntries, namespaceNames),
		Sidecars:               kubernetes.FilterByNamespaceNames(istioConfigs.Sidecars, namespaceNames),
		Telemetries:            kubernetes.FilterByNamespaceNames(istioConfigs.Telemetries, namespaceNames),
		VirtualServices:        kubernetes.FilterByNamespaceNames(istioConfigs.VirtualServices, namespaceNames),
		WasmPlugins:            kubernetes.FilterByNamespaceNames(istioConfigs.WasmPlugins, namespaceNames),
		WorkloadEntries:        kubernetes.FilterByNamespaceNames(istioConfigs.WorkloadEntries, namespaceNames),
		WorkloadGroups:         kubernetes.FilterByNamespaceNames(istioConfigs.WorkloadGroups, namespaceNames),
	}, nil
}

// GetIstioConfigDetails returns a specific Istio configuration object.
// It uses following parameters:
// - "namespace": 		namespace where configuration is stored
// - "objectGVK":		type of the configuration
// - "object":			name of the configuration
func (in *IstioConfigService) GetIstioConfigDetails(ctx context.Context, cluster, namespace string, objectGVK schema.GroupVersionKind, object string) (models.IstioConfigDetails, error) {
	var end observability.EndFunc
	ctx, end = observability.StartSpan(ctx, "GetIstioConfigDetails",
		observability.Attribute("package", "business"),
		observability.Attribute(observability.TracingClusterTag, cluster),
		observability.Attribute("namespace", namespace),
		observability.Attribute("objectGVK", objectGVK.String()),
		observability.Attribute("object", object),
	)
	defer end()

	var err error

	istioConfigDetail := models.IstioConfigDetails{}
	istioConfigDetail.Namespace = models.Namespace{Name: namespace}
	istioConfigDetail.ObjectGVK = objectGVK

	if _, ok := in.userClients[cluster]; !ok {
		return istioConfigDetail, fmt.Errorf("Cluster [%s] is not found or is not accessible for Kiali", cluster)
	}
	// Check if user has access to the namespace (RBAC) in cache scenarios and/or
	// if namespace is accessible from Kiali (Deployment.AccessibleNamespaces)
	if _, err := in.businessLayer.Namespace.GetClusterNamespace(ctx, namespace, cluster); err != nil {
		return istioConfigDetail, err
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func(ctx context.Context) {
		defer wg.Done()
		canCreate, canUpdate, canDelete := getPermissions(ctx, in.userClients[cluster], cluster, namespace, objectGVK, in.conf)
		istioConfigDetail.Permissions = models.ResourcePermissions{
			Create: canCreate,
			Update: canUpdate,
			Delete: canDelete,
		}
	}(ctx)

	getOpts := meta_v1.GetOptions{}

	switch objectGVK {
	case kubernetes.DestinationRules:
		istioConfigDetail.DestinationRule, err = in.userClients[cluster].Istio().NetworkingV1().DestinationRules(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.DestinationRule.Kind = kubernetes.DestinationRules.Kind
			istioConfigDetail.DestinationRule.APIVersion = kubernetes.DestinationRules.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.DestinationRule
		}
	case kubernetes.EnvoyFilters:
		istioConfigDetail.EnvoyFilter, err = in.userClients[cluster].Istio().NetworkingV1alpha3().EnvoyFilters(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.EnvoyFilter.Kind = kubernetes.EnvoyFilters.Kind
			istioConfigDetail.EnvoyFilter.APIVersion = kubernetes.EnvoyFilters.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.EnvoyFilter
		}
	case kubernetes.Gateways:
		istioConfigDetail.Gateway, err = in.userClients[cluster].Istio().NetworkingV1().Gateways(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.Gateway.Kind = kubernetes.Gateways.Kind
			istioConfigDetail.Gateway.APIVersion = kubernetes.Gateways.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.Gateway
		}
	case kubernetes.K8sGateways:
		istioConfigDetail.K8sGateway, err = in.userClients[cluster].GatewayAPI().GatewayV1().Gateways(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.K8sGateway.Kind = kubernetes.K8sGateways.Kind
			istioConfigDetail.K8sGateway.APIVersion = kubernetes.K8sGateways.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.K8sGateway
		}
	case kubernetes.K8sGRPCRoutes:
		istioConfigDetail.K8sGRPCRoute, err = in.userClients[cluster].GatewayAPI().GatewayV1().GRPCRoutes(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.K8sGRPCRoute.Kind = kubernetes.K8sGRPCRoutes.Kind
			istioConfigDetail.K8sGRPCRoute.APIVersion = kubernetes.K8sGRPCRoutes.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.K8sGRPCRoute
		}
	case kubernetes.K8sHTTPRoutes:
		istioConfigDetail.K8sHTTPRoute, err = in.userClients[cluster].GatewayAPI().GatewayV1().HTTPRoutes(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.K8sHTTPRoute.Kind = kubernetes.K8sHTTPRoutes.Kind
			istioConfigDetail.K8sHTTPRoute.APIVersion = kubernetes.K8sHTTPRoutes.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.K8sHTTPRoute
		}
	case kubernetes.K8sInferencePools:
		istioConfigDetail.K8sInferencePool, err = in.userClients[cluster].InferenceAPI().InferenceV1alpha2().InferencePools(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.K8sInferencePool.Kind = kubernetes.K8sInferencePools.Kind
			istioConfigDetail.K8sInferencePool.APIVersion = kubernetes.K8sInferencePools.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.K8sInferencePool
		}
	case kubernetes.K8sReferenceGrants:
		istioConfigDetail.K8sReferenceGrant, err = in.userClients[cluster].GatewayAPI().GatewayV1beta1().ReferenceGrants(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.K8sReferenceGrant.Kind = kubernetes.K8sReferenceGrants.Kind
			istioConfigDetail.K8sReferenceGrant.APIVersion = kubernetes.K8sReferenceGrants.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.K8sReferenceGrant
		}
	case kubernetes.K8sTCPRoutes:
		istioConfigDetail.K8sTCPRoute, err = in.userClients[cluster].GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.K8sTCPRoute.Kind = kubernetes.K8sTCPRoutes.Kind
			istioConfigDetail.K8sTCPRoute.APIVersion = kubernetes.K8sTCPRoutes.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.K8sTCPRoute
		}
	case kubernetes.K8sTLSRoutes:
		istioConfigDetail.K8sTLSRoute, err = in.userClients[cluster].GatewayAPI().GatewayV1alpha2().TLSRoutes(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.K8sTLSRoute.Kind = kubernetes.K8sTLSRoutes.Kind
			istioConfigDetail.K8sTLSRoute.APIVersion = kubernetes.K8sTLSRoutes.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.K8sTLSRoute
		}
	case kubernetes.ServiceEntries:
		istioConfigDetail.ServiceEntry, err = in.userClients[cluster].Istio().NetworkingV1().ServiceEntries(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.ServiceEntry.Kind = kubernetes.ServiceEntries.Kind
			istioConfigDetail.ServiceEntry.APIVersion = kubernetes.ServiceEntries.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.ServiceEntry
		}
	case kubernetes.Sidecars:
		istioConfigDetail.Sidecar, err = in.userClients[cluster].Istio().NetworkingV1().Sidecars(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.Sidecar.Kind = kubernetes.Sidecars.Kind
			istioConfigDetail.Sidecar.APIVersion = kubernetes.Sidecars.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.Sidecar
		}
	case kubernetes.VirtualServices:
		istioConfigDetail.VirtualService, err = in.userClients[cluster].Istio().NetworkingV1().VirtualServices(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.VirtualService.Kind = kubernetes.VirtualServices.Kind
			istioConfigDetail.VirtualService.APIVersion = kubernetes.VirtualServices.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.VirtualService
		}
	case kubernetes.WorkloadEntries:
		istioConfigDetail.WorkloadEntry, err = in.userClients[cluster].Istio().NetworkingV1().WorkloadEntries(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.WorkloadEntry.Kind = kubernetes.WorkloadEntries.Kind
			istioConfigDetail.WorkloadEntry.APIVersion = kubernetes.WorkloadEntries.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.WorkloadEntry
		}
	case kubernetes.WorkloadGroups:
		istioConfigDetail.WorkloadGroup, err = in.userClients[cluster].Istio().NetworkingV1().WorkloadGroups(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.WorkloadGroup.Kind = kubernetes.WorkloadGroups.Kind
			istioConfigDetail.WorkloadGroup.APIVersion = kubernetes.WorkloadGroups.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.WorkloadGroup
		}
	case kubernetes.WasmPlugins:
		istioConfigDetail.WasmPlugin, err = in.userClients[cluster].Istio().ExtensionsV1alpha1().WasmPlugins(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.WasmPlugin.Kind = kubernetes.WasmPlugins.Kind
			istioConfigDetail.WasmPlugin.APIVersion = kubernetes.WasmPlugins.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.WasmPlugin
		}
	case kubernetes.Telemetries:
		istioConfigDetail.Telemetry, err = in.userClients[cluster].Istio().TelemetryV1().Telemetries(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.Telemetry.Kind = kubernetes.Telemetries.Kind
			istioConfigDetail.Telemetry.APIVersion = kubernetes.Telemetries.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.Telemetry
		}
	case kubernetes.AuthorizationPolicies:
		istioConfigDetail.AuthorizationPolicy, err = in.userClients[cluster].Istio().SecurityV1().AuthorizationPolicies(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.AuthorizationPolicy.Kind = kubernetes.AuthorizationPolicies.Kind
			istioConfigDetail.AuthorizationPolicy.APIVersion = kubernetes.AuthorizationPolicies.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.AuthorizationPolicy
		}
	case kubernetes.PeerAuthentications:
		istioConfigDetail.PeerAuthentication, err = in.userClients[cluster].Istio().SecurityV1().PeerAuthentications(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.PeerAuthentication.Kind = kubernetes.PeerAuthentications.Kind
			istioConfigDetail.PeerAuthentication.APIVersion = kubernetes.PeerAuthentications.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.PeerAuthentication
		}
	case kubernetes.RequestAuthentications:
		istioConfigDetail.RequestAuthentication, err = in.userClients[cluster].Istio().SecurityV1().RequestAuthentications(namespace).Get(ctx, object, getOpts)
		if err == nil {
			istioConfigDetail.RequestAuthentication.Kind = kubernetes.RequestAuthentications.Kind
			istioConfigDetail.RequestAuthentication.APIVersion = kubernetes.RequestAuthentications.GroupVersion().String()
			istioConfigDetail.Object = istioConfigDetail.RequestAuthentication
		}
	default:
		err = fmt.Errorf("object type not found: %v", objectGVK.String())
	}

	wg.Wait()

	return istioConfigDetail, err
}

// GetIstioAPI provides the Kubernetes API that manages this Istio resource type
// or empty string if it's not managed
func GetIstioAPI(gvk schema.GroupVersionKind) bool {
	if _, ok := kubernetes.ResourceTypesToAPI[gvk.String()]; ok {
		return true
	}
	return false
}

// DeleteIstioConfigDetail deletes the given Istio resource
func (in *IstioConfigService) DeleteIstioConfigDetail(ctx context.Context, cluster, namespace string, resourceType schema.GroupVersionKind, name string) error {
	var err error
	delOpts := meta_v1.DeleteOptions{}

	userClient := in.userClients[cluster]
	if userClient == nil {
		return fmt.Errorf("K8s Client [%s] is not found or is not accessible for Kiali", cluster)
	}

	details, err := in.GetIstioConfigDetails(ctx, cluster, namespace, resourceType, name)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	switch resourceType {
	case kubernetes.DestinationRules:
		err = userClient.Istio().NetworkingV1().DestinationRules(namespace).Delete(ctx, name, delOpts)
	case kubernetes.EnvoyFilters:
		err = userClient.Istio().NetworkingV1alpha3().EnvoyFilters(namespace).Delete(ctx, name, delOpts)
	case kubernetes.Gateways:
		err = userClient.Istio().NetworkingV1().Gateways(namespace).Delete(ctx, name, delOpts)
	case kubernetes.K8sGateways:
		err = userClient.GatewayAPI().GatewayV1().Gateways(namespace).Delete(ctx, name, delOpts)
	case kubernetes.K8sGRPCRoutes:
		err = userClient.GatewayAPI().GatewayV1().GRPCRoutes(namespace).Delete(ctx, name, delOpts)
	case kubernetes.K8sHTTPRoutes:
		err = userClient.GatewayAPI().GatewayV1().HTTPRoutes(namespace).Delete(ctx, name, delOpts)
	case kubernetes.K8sInferencePools:
		err = userClient.InferenceAPI().InferenceV1alpha2().InferencePools(namespace).Delete(ctx, name, delOpts)
	case kubernetes.K8sReferenceGrants:
		err = userClient.GatewayAPI().GatewayV1beta1().ReferenceGrants(namespace).Delete(ctx, name, delOpts)
	case kubernetes.K8sTCPRoutes:
		err = userClient.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).Delete(ctx, name, delOpts)
	case kubernetes.K8sTLSRoutes:
		err = userClient.GatewayAPI().GatewayV1alpha2().TLSRoutes(namespace).Delete(ctx, name, delOpts)
	case kubernetes.ServiceEntries:
		err = userClient.Istio().NetworkingV1().ServiceEntries(namespace).Delete(ctx, name, delOpts)
	case kubernetes.Sidecars:
		err = userClient.Istio().NetworkingV1().Sidecars(namespace).Delete(ctx, name, delOpts)
	case kubernetes.VirtualServices:
		err = userClient.Istio().NetworkingV1().VirtualServices(namespace).Delete(ctx, name, delOpts)
	case kubernetes.WorkloadEntries:
		err = userClient.Istio().NetworkingV1().WorkloadEntries(namespace).Delete(ctx, name, delOpts)
	case kubernetes.WorkloadGroups:
		err = userClient.Istio().NetworkingV1().WorkloadGroups(namespace).Delete(ctx, name, delOpts)
	case kubernetes.AuthorizationPolicies:
		err = userClient.Istio().SecurityV1().AuthorizationPolicies(namespace).Delete(ctx, name, delOpts)
	case kubernetes.PeerAuthentications:
		err = userClient.Istio().SecurityV1().PeerAuthentications(namespace).Delete(ctx, name, delOpts)
	case kubernetes.RequestAuthentications:
		err = userClient.Istio().SecurityV1().RequestAuthentications(namespace).Delete(ctx, name, delOpts)
	case kubernetes.WasmPlugins:
		err = userClient.Istio().ExtensionsV1alpha1().WasmPlugins(namespace).Delete(ctx, name, delOpts)
	case kubernetes.Telemetries:
		err = userClient.Istio().TelemetryV1().Telemetries(namespace).Delete(ctx, name, delOpts)
	default:
		err = fmt.Errorf("object type not found: %v", resourceType)
	}
	if err != nil {
		return err
	}

	if in.conf.ExternalServices.Istio.IstioAPIEnabled {
		// Refreshing the istio cache in case something has changed with the registry services. Not sure if this is really needed.
		if err := in.controlPlaneMonitor.RefreshIstioCache(ctx); err != nil {
			log.FromContext(ctx).Error().Msgf("Error while refreshing Istio cache: %s", err)
		}
	}

	in.waitForObjectDeletion(ctx, cluster, details.Object)

	// Remove validations for the object to refresh the validation cache.
	in.kialiCache.Validations().Remove(models.IstioValidationKey{Name: name, Namespace: namespace, ObjectGVK: resourceType, Cluster: cluster})

	return nil
}

func (in *IstioConfigService) waitForObjectDeletion(ctx context.Context, cluster string, obj client.Object) {
	kubeCache, err := in.kialiCache.GetKubeCache(cluster)
	if err != nil {
		log.FromContext(ctx).Error().Msgf("Cannot get cache so cannot wait for object to be deleted in cache. You may see stale data but the delete was processed correctly. Error: %s", err)
		return
	}

	if err := kubernetes.WaitForObjectDeleteInCache(ctx, kubeCache, obj); err != nil {
		// It won't break anything if we return the object before it is updated in the cache.
		// We will just show stale data so just log an error here instead of failing.
		log.FromContext(ctx).Error().Msgf("Failed waiting for object to be deleted in cache. You may see stale data but the delete was processed correctly. Error: %s", err)
		return
	}
}

func (in *IstioConfigService) waitForCacheUpdate(ctx context.Context, cluster string, obj client.Object) {
	kubeCache, err := in.kialiCache.GetKubeCache(cluster)
	if err != nil {
		log.FromContext(ctx).Error().Msgf("Cannot get cache so cannot wait for object to update in cache. You may see stale data but the update was processed correctly. Error: %s", err)
		return
	}

	if err := kubernetes.WaitForObjectUpdateInCache(ctx, kubeCache, obj); err != nil {
		// It won't break anything if we return the object before it is updated in the cache.
		// We will just show stale data so just log an error here instead of failing.
		log.FromContext(ctx).Error().Msgf("Failed waiting for object to update in cache. You may see stale data but the update was processed correctly. Error: %s", err)
		return
	}
}

func (in *IstioConfigService) waitForCacheCreate(ctx context.Context, cluster string, obj client.Object) {
	kubeCache, err := in.kialiCache.GetKubeCache(cluster)
	if err != nil {
		log.FromContext(ctx).Error().Msgf("Cannot get cache so cannot wait for object to exist in cache. You may see stale data but the create was processed correctly. Error: %s", err)
		return
	}

	if err := kubernetes.WaitForObjectCreateInCache(ctx, kubeCache, obj); err != nil {
		// It won't break anything if we return the object before it is updated in the cache.
		// We will just show stale data so just log an error here instead of failing.
		log.FromContext(ctx).Error().Msgf("Failed waiting for object to exist in cache. You may see stale data but the create was processed correctly. Error: %s", err)
		return
	}
}

func (in *IstioConfigService) UpdateIstioConfigDetail(ctx context.Context, cluster, namespace string, resourceType schema.GroupVersionKind, name, jsonPatch string) (models.IstioConfigDetails, error) {
	istioConfigDetail := models.IstioConfigDetails{}
	istioConfigDetail.Namespace = models.Namespace{Name: namespace}
	istioConfigDetail.ObjectGVK = resourceType

	patchOpts := meta_v1.PatchOptions{}
	patchType := api_types.MergePatchType
	bytePatch := []byte(jsonPatch)

	userClient := in.userClients[cluster]
	if userClient == nil {
		return istioConfigDetail, fmt.Errorf("K8s Client [%s] is not found or is not accessible for Kiali", cluster)
	}

	var err error

	switch resourceType.String() {
	case kubernetes.DestinationRules.String():
		istioConfigDetail.DestinationRule = &networking_v1.DestinationRule{}
		istioConfigDetail.DestinationRule, err = userClient.Istio().NetworkingV1().DestinationRules(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.DestinationRule
	case kubernetes.EnvoyFilters.String():
		istioConfigDetail.EnvoyFilter = &networking_v1alpha3.EnvoyFilter{}
		istioConfigDetail.EnvoyFilter, err = userClient.Istio().NetworkingV1alpha3().EnvoyFilters(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.EnvoyFilter
	case kubernetes.Gateways.String():
		istioConfigDetail.Gateway = &networking_v1.Gateway{}
		istioConfigDetail.Gateway, err = userClient.Istio().NetworkingV1().Gateways(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.Gateway
	case kubernetes.K8sGateways.String():
		istioConfigDetail.K8sGateway = &k8s_networking_v1.Gateway{}
		istioConfigDetail.K8sGateway, err = userClient.GatewayAPI().GatewayV1().Gateways(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sGateway
	case kubernetes.K8sGRPCRoutes.String():
		istioConfigDetail.K8sGRPCRoute = &k8s_networking_v1.GRPCRoute{}
		istioConfigDetail.K8sGRPCRoute, err = userClient.GatewayAPI().GatewayV1().GRPCRoutes(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sGRPCRoute
	case kubernetes.K8sHTTPRoutes.String():
		istioConfigDetail.K8sHTTPRoute = &k8s_networking_v1.HTTPRoute{}
		istioConfigDetail.K8sHTTPRoute, err = userClient.GatewayAPI().GatewayV1().HTTPRoutes(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sHTTPRoute
	case kubernetes.K8sInferencePools.String():
		istioConfigDetail.K8sInferencePool = &k8s_inference_v1alpha2.InferencePool{}
		istioConfigDetail.K8sInferencePool, err = userClient.InferenceAPI().InferenceV1alpha2().InferencePools(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sInferencePool
	case kubernetes.K8sReferenceGrants.String():
		istioConfigDetail.K8sReferenceGrant = &k8s_networking_v1beta1.ReferenceGrant{}
		fixedPatch := strings.ReplaceAll(jsonPatch, "\"group\":null", "\"group\":\"\"")
		istioConfigDetail.K8sReferenceGrant, err = userClient.GatewayAPI().GatewayV1beta1().ReferenceGrants(namespace).Patch(ctx, name, patchType, []byte(fixedPatch), patchOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sReferenceGrant
	case kubernetes.K8sTCPRoutes.String():
		istioConfigDetail.K8sTCPRoute = &k8s_networking_v1alpha2.TCPRoute{}
		istioConfigDetail.K8sTCPRoute, err = userClient.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sTCPRoute
	case kubernetes.K8sTLSRoutes.String():
		istioConfigDetail.K8sTLSRoute = &k8s_networking_v1alpha2.TLSRoute{}
		istioConfigDetail.K8sTLSRoute, err = userClient.GatewayAPI().GatewayV1alpha2().TLSRoutes(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sTLSRoute
	case kubernetes.ServiceEntries.String():
		istioConfigDetail.ServiceEntry = &networking_v1.ServiceEntry{}
		istioConfigDetail.ServiceEntry, err = userClient.Istio().NetworkingV1().ServiceEntries(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.ServiceEntry
	case kubernetes.Sidecars.String():
		istioConfigDetail.Sidecar = &networking_v1.Sidecar{}
		istioConfigDetail.Sidecar, err = userClient.Istio().NetworkingV1().Sidecars(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.Sidecar
	case kubernetes.VirtualServices.String():
		istioConfigDetail.VirtualService = &networking_v1.VirtualService{}
		istioConfigDetail.VirtualService, err = userClient.Istio().NetworkingV1().VirtualServices(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.VirtualService
	case kubernetes.WorkloadEntries.String():
		istioConfigDetail.WorkloadEntry = &networking_v1.WorkloadEntry{}
		istioConfigDetail.WorkloadEntry, err = userClient.Istio().NetworkingV1().WorkloadEntries(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.WorkloadEntry
	case kubernetes.WorkloadGroups.String():
		istioConfigDetail.WorkloadGroup = &networking_v1.WorkloadGroup{}
		istioConfigDetail.WorkloadGroup, err = userClient.Istio().NetworkingV1().WorkloadGroups(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.WorkloadGroup
	case kubernetes.AuthorizationPolicies.String():
		istioConfigDetail.AuthorizationPolicy = &security_v1.AuthorizationPolicy{}
		istioConfigDetail.AuthorizationPolicy, err = userClient.Istio().SecurityV1().AuthorizationPolicies(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.AuthorizationPolicy
	case kubernetes.PeerAuthentications.String():
		istioConfigDetail.PeerAuthentication = &security_v1.PeerAuthentication{}
		istioConfigDetail.PeerAuthentication, err = userClient.Istio().SecurityV1().PeerAuthentications(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.PeerAuthentication
	case kubernetes.RequestAuthentications.String():
		istioConfigDetail.RequestAuthentication = &security_v1.RequestAuthentication{}
		istioConfigDetail.RequestAuthentication, err = userClient.Istio().SecurityV1().RequestAuthentications(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.RequestAuthentication
	case kubernetes.WasmPlugins.String():
		istioConfigDetail.WasmPlugin = &extentions_v1alpha1.WasmPlugin{}
		istioConfigDetail.WasmPlugin, err = userClient.Istio().ExtensionsV1alpha1().WasmPlugins(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.WasmPlugin
	case kubernetes.Telemetries.String():
		istioConfigDetail.Telemetry = &telemetry_v1.Telemetry{}
		istioConfigDetail.Telemetry, err = userClient.Istio().TelemetryV1().Telemetries(namespace).Patch(ctx, name, patchType, bytePatch, patchOpts)
		istioConfigDetail.Object = istioConfigDetail.Telemetry
	default:
		err = fmt.Errorf("object type not found: %v", resourceType)
	}
	if err != nil {
		return istioConfigDetail, err
	}

	in.waitForCacheUpdate(ctx, cluster, istioConfigDetail.Object)

	// Re-run validations (if enabled) for that object to refresh the validation cache.
	if in.conf.IsValidationsEnabled() {
		if _, _, err := in.businessLayer.Validations.ValidateIstioObject(ctx, cluster, namespace, resourceType, name); err != nil {
			// Logging the error and swallowing it since the object was updated successfully.
			log.FromContext(ctx).Error().Msgf("Error while validating Istio object: %s", err)
		}
	}

	return istioConfigDetail, nil
}

func (in *IstioConfigService) CreateIstioConfigDetail(ctx context.Context, cluster, namespace string, resourceType schema.GroupVersionKind, body []byte) (models.IstioConfigDetails, error) {
	istioConfigDetail := models.IstioConfigDetails{}
	istioConfigDetail.Namespace = models.Namespace{Name: namespace}
	istioConfigDetail.ObjectGVK = resourceType

	createOpts := meta_v1.CreateOptions{}

	userClient := in.userClients[cluster]
	if userClient == nil {
		return istioConfigDetail, fmt.Errorf("K8s Client [%s] is not found or is not accessible for Kiali", cluster)
	}

	var err error

	var name string
	switch resourceType.String() {
	case kubernetes.DestinationRules.String():
		istioConfigDetail.DestinationRule = &networking_v1.DestinationRule{}
		err = json.Unmarshal(body, istioConfigDetail.DestinationRule)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.DestinationRule, err = userClient.Istio().NetworkingV1().DestinationRules(namespace).Create(ctx, istioConfigDetail.DestinationRule, createOpts)
		istioConfigDetail.Object = istioConfigDetail.DestinationRule
		name = istioConfigDetail.DestinationRule.Name
	case kubernetes.EnvoyFilters.String():
		istioConfigDetail.EnvoyFilter = &networking_v1alpha3.EnvoyFilter{}
		err = json.Unmarshal(body, istioConfigDetail.EnvoyFilter)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.EnvoyFilter, err = userClient.Istio().NetworkingV1alpha3().EnvoyFilters(namespace).Create(ctx, istioConfigDetail.EnvoyFilter, createOpts)
		istioConfigDetail.Object = istioConfigDetail.EnvoyFilter
		name = istioConfigDetail.EnvoyFilter.Name
	case kubernetes.Gateways.String():
		istioConfigDetail.Gateway = &networking_v1.Gateway{}
		err = json.Unmarshal(body, istioConfigDetail.Gateway)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.Gateway, err = userClient.Istio().NetworkingV1().Gateways(namespace).Create(ctx, istioConfigDetail.Gateway, createOpts)
		istioConfigDetail.Object = istioConfigDetail.Gateway
		name = istioConfigDetail.Gateway.Name
	case kubernetes.K8sGateways.String():
		istioConfigDetail.K8sGateway = &k8s_networking_v1.Gateway{}
		err = json.Unmarshal(body, istioConfigDetail.K8sGateway)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.K8sGateway, err = userClient.GatewayAPI().GatewayV1().Gateways(namespace).Create(ctx, istioConfigDetail.K8sGateway, createOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sGateway
		name = istioConfigDetail.K8sGateway.Name
	case kubernetes.K8sHTTPRoutes.String():
		istioConfigDetail.K8sHTTPRoute = &k8s_networking_v1.HTTPRoute{}
		err = json.Unmarshal(body, istioConfigDetail.K8sHTTPRoute)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.K8sHTTPRoute, err = userClient.GatewayAPI().GatewayV1().HTTPRoutes(namespace).Create(ctx, istioConfigDetail.K8sHTTPRoute, createOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sHTTPRoute
		name = istioConfigDetail.K8sHTTPRoute.Name
	case kubernetes.K8sInferencePools.String():
		istioConfigDetail.K8sInferencePool = &k8s_inference_v1alpha2.InferencePool{}
		err = json.Unmarshal(body, istioConfigDetail.K8sInferencePool)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.K8sInferencePool, err = userClient.InferenceAPI().InferenceV1alpha2().InferencePools(namespace).Create(ctx, istioConfigDetail.K8sInferencePool, createOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sInferencePool
		name = istioConfigDetail.K8sInferencePool.Name
	case kubernetes.K8sGRPCRoutes.String():
		istioConfigDetail.K8sGRPCRoute = &k8s_networking_v1.GRPCRoute{}
		err = json.Unmarshal(body, istioConfigDetail.K8sGRPCRoute)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.K8sGRPCRoute, err = userClient.GatewayAPI().GatewayV1().GRPCRoutes(namespace).Create(ctx, istioConfigDetail.K8sGRPCRoute, createOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sGRPCRoute
		name = istioConfigDetail.K8sGRPCRoute.Name
	case kubernetes.K8sReferenceGrants.String():
		istioConfigDetail.K8sReferenceGrant = &k8s_networking_v1beta1.ReferenceGrant{}
		err = json.Unmarshal(body, istioConfigDetail.K8sReferenceGrant)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.K8sReferenceGrant, err = userClient.GatewayAPI().GatewayV1beta1().ReferenceGrants(namespace).Create(ctx, istioConfigDetail.K8sReferenceGrant, createOpts)
		istioConfigDetail.Object = istioConfigDetail.K8sReferenceGrant
		name = istioConfigDetail.K8sReferenceGrant.Name
	case kubernetes.ServiceEntries.String():
		istioConfigDetail.ServiceEntry = &networking_v1.ServiceEntry{}
		err = json.Unmarshal(body, istioConfigDetail.ServiceEntry)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.ServiceEntry, err = userClient.Istio().NetworkingV1().ServiceEntries(namespace).Create(ctx, istioConfigDetail.ServiceEntry, createOpts)
		istioConfigDetail.Object = istioConfigDetail.ServiceEntry
		name = istioConfigDetail.ServiceEntry.Name
	case kubernetes.Sidecars.String():
		istioConfigDetail.Sidecar = &networking_v1.Sidecar{}
		err = json.Unmarshal(body, istioConfigDetail.Sidecar)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.Sidecar, err = userClient.Istio().NetworkingV1().Sidecars(namespace).Create(ctx, istioConfigDetail.Sidecar, createOpts)
		istioConfigDetail.Object = istioConfigDetail.Sidecar
		name = istioConfigDetail.Sidecar.Name
	case kubernetes.VirtualServices.String():
		istioConfigDetail.VirtualService = &networking_v1.VirtualService{}
		err = json.Unmarshal(body, istioConfigDetail.VirtualService)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.VirtualService, err = userClient.Istio().NetworkingV1().VirtualServices(namespace).Create(ctx, istioConfigDetail.VirtualService, createOpts)
		istioConfigDetail.Object = istioConfigDetail.VirtualService
		name = istioConfigDetail.VirtualService.Name
	case kubernetes.WorkloadEntries.String():
		istioConfigDetail.WorkloadEntry = &networking_v1.WorkloadEntry{}
		err = json.Unmarshal(body, istioConfigDetail.WorkloadEntry)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.WorkloadEntry, err = userClient.Istio().NetworkingV1().WorkloadEntries(namespace).Create(ctx, istioConfigDetail.WorkloadEntry, createOpts)
		istioConfigDetail.Object = istioConfigDetail.WorkloadEntry
		name = istioConfigDetail.WorkloadEntry.Name
	case kubernetes.WorkloadGroups.String():
		istioConfigDetail.WorkloadGroup = &networking_v1.WorkloadGroup{}
		err = json.Unmarshal(body, istioConfigDetail.WorkloadGroup)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.WorkloadGroup, err = userClient.Istio().NetworkingV1().WorkloadGroups(namespace).Create(ctx, istioConfigDetail.WorkloadGroup, createOpts)
		istioConfigDetail.Object = istioConfigDetail.WorkloadGroup
		name = istioConfigDetail.WorkloadGroup.Name
	case kubernetes.WasmPlugins.String():
		istioConfigDetail.WasmPlugin = &extentions_v1alpha1.WasmPlugin{}
		err = json.Unmarshal(body, istioConfigDetail.WasmPlugin)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.WasmPlugin, err = userClient.Istio().ExtensionsV1alpha1().WasmPlugins(namespace).Create(ctx, istioConfigDetail.WasmPlugin, createOpts)
		istioConfigDetail.Object = istioConfigDetail.WasmPlugin
		name = istioConfigDetail.WasmPlugin.Name
	case kubernetes.Telemetries.String():
		istioConfigDetail.Telemetry = &telemetry_v1.Telemetry{}
		err = json.Unmarshal(body, istioConfigDetail.Telemetry)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.Telemetry, err = userClient.Istio().TelemetryV1().Telemetries(namespace).Create(ctx, istioConfigDetail.Telemetry, createOpts)
		istioConfigDetail.Object = istioConfigDetail.Telemetry
		name = istioConfigDetail.Telemetry.Name
	case kubernetes.AuthorizationPolicies.String():
		istioConfigDetail.AuthorizationPolicy = &security_v1.AuthorizationPolicy{}
		err = json.Unmarshal(body, istioConfigDetail.AuthorizationPolicy)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.AuthorizationPolicy, err = userClient.Istio().SecurityV1().AuthorizationPolicies(namespace).Create(ctx, istioConfigDetail.AuthorizationPolicy, createOpts)
		istioConfigDetail.Object = istioConfigDetail.AuthorizationPolicy
		name = istioConfigDetail.AuthorizationPolicy.Name
	case kubernetes.PeerAuthentications.String():
		istioConfigDetail.PeerAuthentication = &security_v1.PeerAuthentication{}
		err = json.Unmarshal(body, istioConfigDetail.PeerAuthentication)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.PeerAuthentication, err = userClient.Istio().SecurityV1().PeerAuthentications(namespace).Create(ctx, istioConfigDetail.PeerAuthentication, createOpts)
		istioConfigDetail.Object = istioConfigDetail.PeerAuthentication
		name = istioConfigDetail.PeerAuthentication.Name
	case kubernetes.RequestAuthentications.String():
		istioConfigDetail.RequestAuthentication = &security_v1.RequestAuthentication{}
		err = json.Unmarshal(body, istioConfigDetail.RequestAuthentication)
		if err != nil {
			return istioConfigDetail, api_errors.NewBadRequest(err.Error())
		}
		istioConfigDetail.RequestAuthentication, err = userClient.Istio().SecurityV1().RequestAuthentications(namespace).Create(ctx, istioConfigDetail.RequestAuthentication, createOpts)
		istioConfigDetail.Object = istioConfigDetail.RequestAuthentication
		name = istioConfigDetail.RequestAuthentication.Name
	default:
		err = fmt.Errorf("object type not found: %v", resourceType)
	}
	if err != nil {
		return istioConfigDetail, err
	}

	if in.conf.ExternalServices.Istio.IstioAPIEnabled {
		// Refreshing the istio cache in case something has changed with the registry services. Not sure if this is really needed.
		if err := in.controlPlaneMonitor.RefreshIstioCache(ctx); err != nil {
			log.FromContext(ctx).Error().Msgf("Error while refreshing Istio cache: %s", err)
		}
	}

	in.waitForCacheCreate(ctx, cluster, istioConfigDetail.Object)

	// Re-run validations for that object to refresh the validation cache.
	if _, _, err := in.businessLayer.Validations.ValidateIstioObject(ctx, cluster, namespace, resourceType, name); err != nil {
		// Logging the error and swallowing it since the object was created successfully.
		log.FromContext(ctx).Error().Msgf("Error while validating Istio object: %s", err)
	}

	return istioConfigDetail, nil
}

func (in *IstioConfigService) GetIstioConfigPermissions(ctx context.Context, namespaces []string, cluster string) models.IstioConfigPermissions {
	var end observability.EndFunc
	ctx, end = observability.StartSpan(ctx, "GetIstioConfigPermissions",
		observability.Attribute("package", "business"),
		observability.Attribute("namespaces", namespaces),
	)
	defer end()

	istioConfigPermissions := make(models.IstioConfigPermissions, len(namespaces))

	k8s, ok := in.userClients[cluster]
	if !ok {
		log.FromContext(ctx).Error().Msgf("Cluster [%s] doesn't exist ", cluster)
		return nil
	}

	if len(namespaces) > 0 {
		networkingPermissions := make(models.IstioConfigPermissions, len(namespaces))
		k8sNetworkingPermissions := make(models.IstioConfigPermissions, len(namespaces))
		securityPermissions := make(models.IstioConfigPermissions, len(namespaces))

		wg := sync.WaitGroup{}
		// We will query 2 times per namespace (networking.istio.io and security.istio.io)
		wg.Add(len(namespaces) * 3)
		for _, ns := range namespaces {
			networkingRP := make(models.ResourcesPermissions, len(newNetworkingConfigTypes))
			k8sNetworkingRP := make(models.ResourcesPermissions, len(newK8sNetworkingConfigTypes))
			securityRP := make(models.ResourcesPermissions, len(newSecurityConfigTypes))
			networkingPermissions[ns] = &networkingRP
			k8sNetworkingPermissions[ns] = &k8sNetworkingRP
			securityPermissions[ns] = &securityRP
			/*
				We can optimize this logic.
				Instead of query all editable objects of networking.istio.io and security.istio.io we can query
				only one per API, that will save several queries to the backend.

				Synced with:
				https://github.com/kiali/kiali-operator/blob/master/roles/default/kiali-deploy/templates/kubernetes/role.yaml#L62
			*/
			go func(ctx context.Context, namespace string, wg *sync.WaitGroup, networkingPermissions *models.ResourcesPermissions) {
				defer wg.Done()
				canCreate, canUpdate, canDelete := getPermissionsApi(ctx, k8s, cluster, namespace, kubernetes.NetworkingGroupVersionV1.Group, allResources, in.conf)
				for _, rs := range newNetworkingConfigTypes {
					networkingRP[rs.String()] = &models.ResourcePermissions{
						Create: canCreate,
						Update: canUpdate,
						Delete: canDelete,
					}
				}
			}(ctx, ns, &wg, &networkingRP)

			go func(ctx context.Context, namespace string, wg *sync.WaitGroup, k8sNetworkingPermissions *models.ResourcesPermissions) {
				defer wg.Done()
				canCreate, canUpdate, canDelete := getPermissionsApi(ctx, k8s, cluster, namespace, kubernetes.K8sNetworkingGroupVersionV1.Group, allResources, in.conf)
				for _, rs := range newK8sNetworkingConfigTypes {
					k8sNetworkingRP[rs.String()] = &models.ResourcePermissions{
						Create: canCreate && in.userClients[cluster].IsGatewayAPI(),
						Update: canUpdate && in.userClients[cluster].IsGatewayAPI(),
						Delete: canDelete && in.userClients[cluster].IsGatewayAPI(),
					}
				}
			}(ctx, ns, &wg, &k8sNetworkingRP)

			go func(ctx context.Context, namespace string, wg *sync.WaitGroup, securityPermissions *models.ResourcesPermissions) {
				defer wg.Done()
				canCreate, canUpdate, canDelete := getPermissionsApi(ctx, k8s, cluster, namespace, kubernetes.SecurityGroupVersionV1.Group, allResources, in.conf)
				for _, rs := range newSecurityConfigTypes {
					securityRP[rs.String()] = &models.ResourcePermissions{
						Create: canCreate,
						Update: canUpdate,
						Delete: canDelete,
					}
				}
			}(ctx, ns, &wg, &securityRP)
		}
		wg.Wait()

		// Join networking and security permissions into a single result
		for _, ns := range namespaces {
			allRP := make(models.ResourcesPermissions, len(newNetworkingConfigTypes)+len(newSecurityConfigTypes)+len(newK8sNetworkingConfigTypes))
			istioConfigPermissions[ns] = &allRP
			for resource, permissions := range *networkingPermissions[ns] {
				(*istioConfigPermissions[ns])[resource] = permissions
			}
			for resource, permissions := range *k8sNetworkingPermissions[ns] {
				(*istioConfigPermissions[ns])[resource] = permissions
			}
			for resource, permissions := range *securityPermissions[ns] {
				(*istioConfigPermissions[ns])[resource] = permissions
			}
		}
	}
	return istioConfigPermissions
}

func getPermissions(ctx context.Context, k8s kubernetes.ClientInterface, cluster string, namespace string, objectGVK schema.GroupVersionKind, conf *config.Config) (bool, bool, bool) {
	var canCreate, canPatch, canDelete bool

	if _, ok := kubernetes.ResourceTypesToAPI[objectGVK.String()]; ok {
		return getPermissionsApi(ctx, k8s, cluster, namespace, objectGVK.Group, objectGVK.Kind, conf)
	}
	return canCreate, canPatch, canDelete
}

func getPermissionsApi(ctx context.Context, k8s kubernetes.ClientInterface, cluster string, namespace, api, resourceType string, conf *config.Config) (bool, bool, bool) {
	var canCreate, canPatch, canDelete bool

	// In view only mode, there is not need to check RBAC permissions, return false early
	if conf.Deployment.ViewOnlyMode {
		log.FromContext(ctx).Debug().Msg("View only mode configured, skipping RBAC checks")
		return canCreate, canPatch, canDelete
	}

	/*
		Kiali only uses create,patch,delete as WRITE permissions

		"update" creates an extra call to the API that we know that it will always fail, introducing extra latency

		Synced with:
		https://github.com/kiali/kiali-operator/blob/master/roles/default/kiali-deploy/templates/kubernetes/role.yaml#L62
	*/
	ssars, permErr := k8s.GetSelfSubjectAccessReview(ctx, namespace, api, resourceType, []string{"create", "patch", "delete"})
	if permErr == nil {
		for _, ssar := range ssars {
			if ssar.Spec.ResourceAttributes != nil {
				switch ssar.Spec.ResourceAttributes.Verb {
				case "create":
					canCreate = ssar.Status.Allowed
				case "patch":
					canPatch = ssar.Status.Allowed
				case "delete":
					canDelete = ssar.Status.Allowed
				}
			}
		}
	} else {
		log.FromContext(ctx).Error().Msgf("Error getting permissions [namespace: %s, api: %s, resourceType: %s]: %v", namespace, api, "*", permErr)
	}
	return canCreate, canPatch, canDelete
}

func checkType(types []string, name string) bool {
	for _, typeName := range types {
		if typeName == name {
			return true
		}
	}
	return false
}

func ParseIstioConfigCriteria(objects, labelSelector, workloadSelector string) IstioConfigCriteria {
	defaultInclude := objects == ""
	criteria := IstioConfigCriteria{}
	criteria.IncludeGateways = defaultInclude
	criteria.IncludeK8sGateways = defaultInclude
	criteria.IncludeK8sGRPCRoutes = defaultInclude
	criteria.IncludeK8sHTTPRoutes = defaultInclude
	criteria.IncludeK8sInferencePools = defaultInclude
	criteria.IncludeK8sReferenceGrants = defaultInclude
	criteria.IncludeK8sTCPRoutes = defaultInclude
	criteria.IncludeK8sTLSRoutes = defaultInclude
	criteria.IncludeVirtualServices = defaultInclude
	criteria.IncludeDestinationRules = defaultInclude
	criteria.IncludeServiceEntries = defaultInclude
	criteria.IncludeSidecars = defaultInclude
	criteria.IncludeAuthorizationPolicies = defaultInclude
	criteria.IncludePeerAuthentications = defaultInclude
	criteria.IncludeWorkloadEntries = defaultInclude
	criteria.IncludeWorkloadGroups = defaultInclude
	criteria.IncludeRequestAuthentications = defaultInclude
	criteria.IncludeEnvoyFilters = defaultInclude
	criteria.IncludeWasmPlugins = defaultInclude
	criteria.IncludeTelemetry = defaultInclude
	criteria.LabelSelector = labelSelector
	criteria.WorkloadSelector = workloadSelector

	if defaultInclude {
		return criteria
	}

	types := strings.Split(objects, ";")
	if checkType(types, kubernetes.Gateways.String()) {
		criteria.IncludeGateways = true
	}
	if checkType(types, kubernetes.K8sGateways.String()) {
		criteria.IncludeK8sGateways = true
	}
	if checkType(types, kubernetes.K8sGRPCRoutes.String()) {
		criteria.IncludeK8sGRPCRoutes = true
	}
	if checkType(types, kubernetes.K8sHTTPRoutes.String()) {
		criteria.IncludeK8sHTTPRoutes = true
	}
	if checkType(types, kubernetes.K8sInferencePools.String()) {
		criteria.IncludeK8sInferencePools = true
	}
	if checkType(types, kubernetes.K8sReferenceGrants.String()) {
		criteria.IncludeK8sReferenceGrants = true
	}
	if checkType(types, kubernetes.K8sTCPRoutes.String()) {
		criteria.IncludeK8sTCPRoutes = true
	}
	if checkType(types, kubernetes.K8sTLSRoutes.String()) {
		criteria.IncludeK8sTLSRoutes = true
	}
	if checkType(types, kubernetes.VirtualServices.String()) {
		criteria.IncludeVirtualServices = true
	}
	if checkType(types, kubernetes.DestinationRules.String()) {
		criteria.IncludeDestinationRules = true
	}
	if checkType(types, kubernetes.ServiceEntries.String()) {
		criteria.IncludeServiceEntries = true
	}
	if checkType(types, kubernetes.Sidecars.String()) {
		criteria.IncludeSidecars = true
	}
	if checkType(types, kubernetes.AuthorizationPolicies.String()) {
		criteria.IncludeAuthorizationPolicies = true
	}
	if checkType(types, kubernetes.PeerAuthentications.String()) {
		criteria.IncludePeerAuthentications = true
	}
	if checkType(types, kubernetes.WorkloadEntries.String()) {
		criteria.IncludeWorkloadEntries = true
	}
	if checkType(types, kubernetes.WorkloadGroups.String()) {
		criteria.IncludeWorkloadGroups = true
	}
	if checkType(types, kubernetes.WasmPlugins.String()) {
		criteria.IncludeWasmPlugins = true
	}
	if checkType(types, kubernetes.Telemetries.String()) {
		criteria.IncludeTelemetry = true
	}
	if checkType(types, kubernetes.RequestAuthentications.String()) {
		criteria.IncludeRequestAuthentications = true
	}
	if checkType(types, kubernetes.EnvoyFilters.String()) {
		criteria.IncludeEnvoyFilters = true
	}
	return criteria
}
