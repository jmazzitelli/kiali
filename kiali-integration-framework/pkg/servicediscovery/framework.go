package servicediscovery

import (
	"context"
	"fmt"

	"github.com/kiali/kiali-integration-framework/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Framework manages service discovery providers and configurations
type Framework struct {
	logger    *utils.Logger
	providers map[ServiceDiscoveryType]ServiceDiscoveryProvider
}

// NewFramework creates a new service discovery framework
func NewFramework() *Framework {
	return &Framework{
		logger:    utils.GetGlobalLogger(),
		providers: make(map[ServiceDiscoveryType]ServiceDiscoveryProvider),
	}
}

// RegisterProvider registers a service discovery provider
func (f *Framework) RegisterProvider(provider ServiceDiscoveryProvider) {
	f.providers[provider.Type()] = provider
	f.logger.Infof("Registered service discovery provider: %s", provider.Type())
}

// GetProvider returns a service discovery provider by type
func (f *Framework) GetProvider(discoveryType ServiceDiscoveryType) (ServiceDiscoveryProvider, error) {
	provider, exists := f.providers[discoveryType]
	if !exists {
		return nil, utils.NewFrameworkError(utils.ErrCodeInvalidParameter,
			fmt.Sprintf("service discovery provider not found: %s", discoveryType), nil)
	}
	return provider, nil
}

// InstallServiceDiscovery installs service discovery using a provider
func (f *Framework) InstallServiceDiscovery(ctx context.Context, clientset kubernetes.Interface, discoveryType ServiceDiscoveryType, config map[string]interface{}) error {
	return f.logger.LogOperationWithContext("install service discovery",
		map[string]interface{}{
			"type": discoveryType,
		}, func() error {
			f.logger.Infof("Installing service discovery: %s", discoveryType)

			provider, err := f.GetProvider(discoveryType)
			if err != nil {
				return err
			}

			// Validate the configuration
			if err := provider.ValidateConfig(config); err != nil {
				return err
			}

			// Install using the provider
			if err := provider.Install(ctx, clientset, config); err != nil {
				return err
			}

			f.logger.Infof("Successfully installed service discovery: %s", discoveryType)
			return nil
		})
}

// UninstallServiceDiscovery uninstalls service discovery
func (f *Framework) UninstallServiceDiscovery(ctx context.Context, clientset kubernetes.Interface, discoveryType ServiceDiscoveryType) error {
	return f.logger.LogOperationWithContext("uninstall service discovery",
		map[string]interface{}{
			"type": discoveryType,
		}, func() error {
			f.logger.Infof("Uninstalling service discovery: %s", discoveryType)

			provider, err := f.GetProvider(discoveryType)
			if err != nil {
				return err
			}

			if err := provider.Uninstall(ctx, clientset); err != nil {
				return err
			}

			f.logger.Infof("Successfully uninstalled service discovery: %s", discoveryType)
			return nil
		})
}

// GetServiceDiscoveryStatus gets the status of service discovery
func (f *Framework) GetServiceDiscoveryStatus(ctx context.Context, clientset kubernetes.Interface, discoveryType ServiceDiscoveryType) (ServiceDiscoveryStatus, error) {
	provider, err := f.GetProvider(discoveryType)
	if err != nil {
		return ServiceDiscoveryStatus{}, err
	}

	return provider.Status(ctx, clientset)
}

// RunHealthChecks runs all service discovery health checks
func (f *Framework) RunHealthChecks(ctx context.Context, clientset kubernetes.Interface, discoveryType ServiceDiscoveryType) ([]ServiceDiscoveryHealthCheck, error) {
	provider, err := f.GetProvider(discoveryType)
	if err != nil {
		return nil, err
	}

	return provider.HealthCheck(ctx, clientset)
}

// CreateDNSConfigMap creates a ConfigMap for DNS configuration
func (f *Framework) CreateDNSConfigMap(ctx context.Context, clientset kubernetes.Interface, name, namespace string, dnsConfig *DNSConfig) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
				"service-discovery":            "true",
				"discovery-type":               "dns",
			},
		},
		Data: map[string]string{
			"resolv.conf": f.generateResolvConf(dnsConfig),
			"dns-config": fmt.Sprintf("enabled: %t\nclusters: %v\nsearchDomains: %v\nnameservers: %v\nttl: %d",
				dnsConfig.Enabled, dnsConfig.Clusters, dnsConfig.SearchDomains, dnsConfig.Nameservers, dnsConfig.TTL),
		},
	}

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return utils.WrapError(err, utils.ErrCodeComponentInstallFailed,
			"failed to create DNS config map")
	}

	f.logger.Infof("Created DNS config map: %s/%s", namespace, name)
	return nil
}

// CreateAPIServerConfigMap creates a ConfigMap for API server configuration
func (f *Framework) CreateAPIServerConfigMap(ctx context.Context, clientset kubernetes.Interface, name, namespace string, apiConfig *APIServerConfig) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
				"service-discovery":            "true",
				"discovery-type":               "api-server",
			},
		},
		Data: map[string]string{
			"api-server-config": fmt.Sprintf("enabled: %t\nclusters: %v\napiServerUrl: %s",
				apiConfig.Enabled, apiConfig.Clusters, apiConfig.APIServerURL),
		},
	}

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return utils.WrapError(err, utils.ErrCodeComponentInstallFailed,
			"failed to create API server config map")
	}

	f.logger.Infof("Created API server config map: %s/%s", namespace, name)
	return nil
}

// CreateServicePropagationConfigMap creates a ConfigMap for service propagation configuration
func (f *Framework) CreateServicePropagationConfigMap(ctx context.Context, clientset kubernetes.Interface, name, namespace string, propagationConfig *ServicePropagationConfig) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
				"service-discovery":            "true",
				"discovery-type":               "propagation",
			},
		},
		Data: map[string]string{
			"propagation-config": fmt.Sprintf("enabled: %t\nclusters: %v\nsyncInterval: %v\nnamespaces: %v",
				propagationConfig.Enabled, propagationConfig.Clusters, propagationConfig.SyncInterval, propagationConfig.Namespaces),
		},
	}

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return utils.WrapError(err, utils.ErrCodeComponentInstallFailed,
			"failed to create service propagation config map")
	}

	f.logger.Infof("Created service propagation config map: %s/%s", namespace, name)
	return nil
}

// generateResolvConf generates a resolv.conf file content for DNS configuration
func (f *Framework) generateResolvConf(dnsConfig *DNSConfig) string {
	content := ""

	// Add nameservers
	for _, nameserver := range dnsConfig.Nameservers {
		content += fmt.Sprintf("nameserver %s\n", nameserver)
	}

	// Add search domains
	if len(dnsConfig.SearchDomains) > 0 {
		content += "search"
		for _, domain := range dnsConfig.SearchDomains {
			content += " " + domain
		}
		content += "\n"
	}

	// Add options
	if dnsConfig.TTL > 0 {
		content += fmt.Sprintf("options timeout:%d\n", dnsConfig.TTL)
	}

	return content
}

// ValidateServiceDiscoveryConfig validates service discovery configuration
func (f *Framework) ValidateServiceDiscoveryConfig(config *ServiceDiscoveryConfig) error {
	if !config.Enabled {
		return nil
	}

	if config.Type == "" {
		return fmt.Errorf("service discovery type is required when enabled")
	}

	if len(config.Clusters) == 0 {
		return fmt.Errorf("at least one cluster must be specified for service discovery")
	}

	provider, err := f.GetProvider(config.Type)
	if err != nil {
		return fmt.Errorf("unsupported service discovery type: %s", config.Type)
	}

	return provider.ValidateConfig(config.Config)
}

// GetServiceDiscoveryInfo retrieves service discovery information from a cluster
func (f *Framework) GetServiceDiscoveryInfo(ctx context.Context, clientset kubernetes.Interface, namespace string) ([]ServiceEndpoint, error) {
	services, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, utils.WrapError(err, utils.ErrCodeClusterUnhealthy,
			"failed to list services for discovery info")
	}

	var endpoints []ServiceEndpoint
	for _, svc := range services.Items {
		// Skip services that shouldn't be propagated
		if f.shouldSkipService(&svc) {
			continue
		}

		endpoint := ServiceEndpoint{
			Name:        svc.Name,
			Namespace:   svc.Namespace,
			Type:        string(svc.Spec.Type),
			Ports:       f.convertPorts(svc.Spec.Ports),
			Labels:      svc.Labels,
			Annotations: svc.Annotations,
			LastUpdated: svc.CreationTimestamp.Time,
		}

		endpoints = append(endpoints, endpoint)
	}

	return endpoints, nil
}

// shouldSkipService determines if a service should be skipped during discovery
func (f *Framework) shouldSkipService(svc *corev1.Service) bool {
	// Skip kubernetes default services
	if svc.Name == "kubernetes" {
		return true
	}

	// Skip services with specific labels
	if svc.Labels != nil {
		if svc.Labels["service-discovery/skip"] == "true" {
			return true
		}
	}

	return false
}

// convertPorts converts Kubernetes service ports to our ServicePort type
func (f *Framework) convertPorts(ports []corev1.ServicePort) []ServicePort {
	var result []ServicePort
	for _, port := range ports {
		servicePort := ServicePort{
			Name:     port.Name,
			Port:     port.Port,
			Protocol: string(port.Protocol),
		}

		// Handle different target port types
		if port.TargetPort.Type == 1 { // IntOrString is Int
			servicePort.TargetPort = port.TargetPort.IntVal
		}

		result = append(result, servicePort)
	}
	return result
}
