package connectivity

import (
	"context"
	"fmt"

	"github.com/kiali/kiali-integration-framework/pkg/servicediscovery"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Framework manages connectivity policies and templates
type Framework struct {
	logger           *utils.Logger
	providers        map[ConnectivityType]ConnectivityProvider
	templates        map[string]*ConnectivityTemplate
	serviceDiscovery *servicediscovery.Framework
}

// NewFramework creates a new connectivity framework
func NewFramework() *Framework {
	return &Framework{
		logger:           utils.GetGlobalLogger(),
		providers:        make(map[ConnectivityType]ConnectivityProvider),
		templates:        make(map[string]*ConnectivityTemplate),
		serviceDiscovery: servicediscovery.NewFramework(),
	}
}

// RegisterProvider registers a connectivity provider
func (f *Framework) RegisterProvider(provider ConnectivityProvider) {
	f.providers[provider.Type()] = provider
	f.logger.Infof("Registered connectivity provider: %s", provider.Type())
}

// GetProvider returns a connectivity provider by type
func (f *Framework) GetProvider(connectivityType ConnectivityType) (ConnectivityProvider, error) {
	provider, exists := f.providers[connectivityType]
	if !exists {
		return nil, utils.NewFrameworkError(utils.ErrCodeInvalidParameter,
			fmt.Sprintf("connectivity provider not found: %s", connectivityType), nil)
	}
	return provider, nil
}

// RegisterTemplate registers a connectivity template
func (f *Framework) RegisterTemplate(template *ConnectivityTemplate) {
	f.templates[template.Name] = template
	f.logger.Infof("Registered connectivity template: %s", template.Name)
}

// GetTemplate returns a connectivity template by name
func (f *Framework) GetTemplate(name string) (*ConnectivityTemplate, error) {
	template, exists := f.templates[name]
	if !exists {
		return nil, utils.NewFrameworkError(utils.ErrCodeConfigNotFound,
			fmt.Sprintf("connectivity template not found: %s", name), nil)
	}
	return template, nil
}

// ListTemplates returns all registered templates
func (f *Framework) ListTemplates() map[string]*ConnectivityTemplate {
	return f.templates
}

// InstallConnectivity installs connectivity using a template
func (f *Framework) InstallConnectivity(ctx context.Context, clientset *kubernetes.Clientset, templateName string, config map[string]interface{}) error {
	template, err := f.GetTemplate(templateName)
	if err != nil {
		return err
	}

	return f.logger.LogOperationWithContext("install connectivity template",
		map[string]interface{}{
			"template": templateName,
			"type":     template.Type,
		}, func() error {
			f.logger.Infof("Installing connectivity template: %s", templateName)

			provider, err := f.GetProvider(template.Type)
			if err != nil {
				return err
			}

			// Merge template config with provided config
			mergedConfig := f.mergeConfigs(template.Config, config)

			// Validate the merged configuration
			if err := provider.ValidateConfig(mergedConfig); err != nil {
				return err
			}

			// Install using the provider
			if err := provider.Install(ctx, clientset, mergedConfig); err != nil {
				return err
			}

			f.logger.Infof("Successfully installed connectivity template: %s", templateName)
			return nil
		})
}

// UninstallConnectivity uninstalls connectivity
func (f *Framework) UninstallConnectivity(ctx context.Context, clientset *kubernetes.Clientset, connectivityType ConnectivityType) error {
	return f.logger.LogOperationWithContext("uninstall connectivity",
		map[string]interface{}{
			"type": connectivityType,
		}, func() error {
			f.logger.Infof("Uninstalling connectivity: %s", connectivityType)

			provider, err := f.GetProvider(connectivityType)
			if err != nil {
				return err
			}

			if err := provider.Uninstall(ctx, clientset); err != nil {
				return err
			}

			f.logger.Infof("Successfully uninstalled connectivity: %s", connectivityType)
			return nil
		})
}

// GetConnectivityStatus gets the status of connectivity
func (f *Framework) GetConnectivityStatus(ctx context.Context, clientset *kubernetes.Clientset, connectivityType ConnectivityType) (ConnectivityStatus, error) {
	provider, err := f.GetProvider(connectivityType)
	if err != nil {
		return ConnectivityStatus{}, err
	}

	return provider.Status(ctx, clientset)
}

// RunHealthChecks runs all connectivity health checks
func (f *Framework) RunHealthChecks(ctx context.Context, clientset *kubernetes.Clientset, connectivityType ConnectivityType) ([]ConnectivityHealthCheck, error) {
	provider, err := f.GetProvider(connectivityType)
	if err != nil {
		return nil, err
	}

	return provider.HealthCheck(ctx, clientset)
}

// CreateNetworkPolicy creates a basic network policy for cross-cluster communication
func (f *Framework) CreateNetworkPolicy(ctx context.Context, clientset *kubernetes.Clientset, name, namespace string, allowCIDRs []string) error {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
				"connectivity-framework":       "true",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // Allow all pods
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{},
				},
			},
		},
	}

	// Add CIDR blocks to both ingress and egress rules
	for _, cidr := range allowCIDRs {
		peer := networkingv1.NetworkPolicyPeer{
			IPBlock: &networkingv1.IPBlock{
				CIDR: cidr,
			},
		}

		networkPolicy.Spec.Ingress[0].From = append(networkPolicy.Spec.Ingress[0].From, peer)
		networkPolicy.Spec.Egress[0].To = append(networkPolicy.Spec.Egress[0].To, peer)
	}

	_, err := clientset.NetworkingV1().NetworkPolicies(namespace).Create(ctx, networkPolicy, metav1.CreateOptions{})
	if err != nil {
		return utils.WrapError(err, utils.ErrCodeComponentInstallFailed,
			"failed to create network policy")
	}

	f.logger.Infof("Created network policy: %s/%s", namespace, name)
	return nil
}

// CreateServiceEntry creates a service entry for cross-cluster service discovery
func (f *Framework) CreateServiceEntry(ctx context.Context, clientset *kubernetes.Clientset, name, namespace string, hosts []string, ports []corev1.ServicePort) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
				"connectivity-framework":       "true",
				"connectivity-type":            "service-discovery",
			},
			Annotations: map[string]string{
				"connectivity-framework/hosts": fmt.Sprintf("%v", hosts),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeClusterIP,
			Ports: ports,
		},
	}

	_, err := clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return utils.WrapError(err, utils.ErrCodeComponentInstallFailed,
			"failed to create service entry")
	}

	f.logger.Infof("Created service entry: %s/%s", namespace, name)
	return nil
}

// CreateConfigMap creates a configuration map for connectivity settings
func (f *Framework) CreateConfigMap(ctx context.Context, clientset *kubernetes.Clientset, name, namespace string, data map[string]string) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
				"connectivity-framework":       "true",
			},
		},
		Data: data,
	}

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return utils.WrapError(err, utils.ErrCodeComponentInstallFailed,
			"failed to create config map")
	}

	f.logger.Infof("Created config map: %s/%s", namespace, name)
	return nil
}

// mergeConfigs merges two configuration maps
func (f *Framework) mergeConfigs(base, override map[string]interface{}) map[string]interface{} {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	result := make(map[string]interface{})

	// Copy base config
	for k, v := range base {
		result[k] = v
	}

	// Override with provided config
	for k, v := range override {
		if vMap, ok := v.(map[string]interface{}); ok {
			if baseMap, exists := result[k]; exists {
				if baseMapVal, ok := baseMap.(map[string]interface{}); ok {
					result[k] = f.mergeConfigs(baseMapVal, vMap)
					continue
				}
			}
		}
		result[k] = v
	}

	return result
}

// GetServiceDiscoveryFramework returns the service discovery framework
func (f *Framework) GetServiceDiscoveryFramework() *servicediscovery.Framework {
	return f.serviceDiscovery
}

// RegisterServiceDiscoveryProvider registers a service discovery provider
func (f *Framework) RegisterServiceDiscoveryProvider(provider servicediscovery.ServiceDiscoveryProvider) {
	f.serviceDiscovery.RegisterProvider(provider)
	f.logger.Infof("Registered service discovery provider: %s", provider.Type())
}

// InitializeDefaultTemplates initializes the framework with default connectivity templates
func (f *Framework) InitializeDefaultTemplates() {
	// Kubernetes basic connectivity template
	k8sTemplate := &ConnectivityTemplate{
		Name:        "kubernetes-basic",
		Type:        ConnectivityTypeKubernetes,
		Description: "Basic Kubernetes connectivity for cross-cluster communication",
		Version:     "1.0.0",
		Policies: []ConnectivityPolicy{
			{
				Name:        "allow-cross-cluster-traffic",
				Type:        ConnectivityTypeKubernetes,
				Version:     "1.0.0",
				Enabled:     true,
				Description: "Allow traffic between cluster CIDRs",
				Config: map[string]interface{}{
					"allowCIDRs": []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
				},
			},
		},
		Config: map[string]interface{}{
			"networkPolicies":  true,
			"serviceDiscovery": true,
			"dns": map[string]interface{}{
				"enabled":       true,
				"searchDomains": []string{"cluster.local"},
			},
		},
		Tags: []string{"kubernetes", "basic", "networking"},
	}

	// Istio service mesh connectivity template
	istioTemplate := &ConnectivityTemplate{
		Name:        "istio-service-mesh",
		Type:        ConnectivityTypeIstio,
		Description: "Istio service mesh connectivity for advanced traffic management",
		Version:     "1.0.0",
		Policies: []ConnectivityPolicy{
			{
				Name:        "istio-traffic-management",
				Type:        ConnectivityTypeIstio,
				Version:     "1.0.0",
				Enabled:     true,
				Description: "Configure Istio for cross-cluster traffic management",
				Config: map[string]interface{}{
					"trafficPolicies":  true,
					"destinationRules": true,
					"virtualServices":  true,
				},
			},
		},
		Config: map[string]interface{}{
			"serviceMesh": map[string]interface{}{
				"enabled": true,
				"discoverySelectors": []map[string]string{
					{"istio.io/tag": "cross-cluster"},
				},
			},
			"trafficManagement": map[string]interface{}{
				"loadBalancing":  "ROUND_ROBIN",
				"circuitBreaker": true,
			},
		},
		Tags: []string{"istio", "service-mesh", "traffic-management"},
	}

	// Linkerd service mesh connectivity template
	linkerdTemplate := &ConnectivityTemplate{
		Name:        "linkerd-service-mesh",
		Type:        ConnectivityTypeLinkerd,
		Description: "Linkerd service mesh connectivity for lightweight traffic management",
		Version:     "1.0.0",
		Policies: []ConnectivityPolicy{
			{
				Name:        "linkerd-traffic-splitting",
				Type:        ConnectivityTypeLinkerd,
				Version:     "1.0.0",
				Enabled:     true,
				Description: "Configure Linkerd for cross-cluster traffic splitting",
				Config: map[string]interface{}{
					"trafficSplit":    true,
					"serviceProfiles": true,
				},
			},
		},
		Config: map[string]interface{}{
			"serviceMesh": map[string]interface{}{
				"enabled": true,
			},
			"trafficManagement": map[string]interface{}{
				"loadBalancing": "ewma",
				"retries":       3,
			},
		},
		Tags: []string{"linkerd", "service-mesh", "lightweight"},
	}

	// Manual connectivity template
	manualTemplate := &ConnectivityTemplate{
		Name:        "manual-configuration",
		Type:        ConnectivityTypeManual,
		Description: "Manual connectivity configuration for custom setups",
		Version:     "1.0.0",
		Policies: []ConnectivityPolicy{
			{
				Name:        "custom-connectivity",
				Type:        ConnectivityTypeManual,
				Version:     "1.0.0",
				Enabled:     true,
				Description: "Apply custom connectivity configuration",
				Config: map[string]interface{}{
					"customConfig": true,
				},
			},
		},
		Config: map[string]interface{}{
			"manual": map[string]interface{}{
				"enabled":        true,
				"validateConfig": true,
			},
		},
		Tags: []string{"manual", "custom", "configuration"},
	}

	f.RegisterTemplate(k8sTemplate)
	f.RegisterTemplate(istioTemplate)
	f.RegisterTemplate(linkerdTemplate)
	f.RegisterTemplate(manualTemplate)

	f.logger.Info("Initialized default connectivity templates")
}

// InitializeServiceDiscoveryProviders initializes the framework with default service discovery providers
func (f *Framework) InitializeServiceDiscoveryProviders() {
	// Register DNS provider
	dnsProvider := servicediscovery.NewDNSProvider()
	f.RegisterServiceDiscoveryProvider(dnsProvider)

	// Register API server provider
	apiServerProvider := servicediscovery.NewAPIServerProvider()
	f.RegisterServiceDiscoveryProvider(apiServerProvider)

	// Register propagation provider
	propagationProvider := servicediscovery.NewPropagationProvider()
	f.RegisterServiceDiscoveryProvider(propagationProvider)

	// Register manual provider
	manualProvider := servicediscovery.NewManualProvider()
	f.RegisterServiceDiscoveryProvider(manualProvider)

	f.logger.Info("Initialized default service discovery providers")
}
