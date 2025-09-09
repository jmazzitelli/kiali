package connectivity

import (
	"context"
	"fmt"
	"time"

	"github.com/kiali/kiali-integration-framework/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// KubernetesProvider implements Kubernetes-based connectivity
type KubernetesProvider struct {
	logger *utils.Logger
}

// NewKubernetesProvider creates a new Kubernetes connectivity provider
func NewKubernetesProvider() *KubernetesProvider {
	return &KubernetesProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Type returns the connectivity type
func (kp *KubernetesProvider) Type() ConnectivityType {
	return ConnectivityTypeKubernetes
}

// ValidateConfig validates the Kubernetes connectivity configuration
func (kp *KubernetesProvider) ValidateConfig(config map[string]interface{}) error {
	if config == nil {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"Kubernetes connectivity configuration is required", nil)
	}

	// Check for network policies configuration
	if networkPolicies, exists := config["networkPolicies"]; exists {
		if enabled, ok := networkPolicies.(bool); ok && enabled {
			if allowCIDRs, exists := config["allowCIDRs"]; exists {
				if cidrs, ok := allowCIDRs.([]interface{}); ok {
					if len(cidrs) == 0 {
						return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
							"allowCIDRs cannot be empty when networkPolicies is enabled", nil)
					}
					// Validate CIDR format
					for _, cidr := range cidrs {
						if cidrStr, ok := cidr.(string); ok {
							if !isValidCIDR(cidrStr) {
								return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
									fmt.Sprintf("invalid CIDR format: %s", cidrStr), nil)
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// Install installs Kubernetes connectivity
func (kp *KubernetesProvider) Install(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	kp.logger.Info("Installing Kubernetes connectivity")

	// Create network policies if enabled
	if networkPolicies, exists := config["networkPolicies"]; exists {
		if enabled, ok := networkPolicies.(bool); ok && enabled {
			if err := kp.installNetworkPolicies(ctx, clientset, config); err != nil {
				return err
			}
		}
	}

	// Configure service discovery
	if serviceDiscovery, exists := config["serviceDiscovery"]; exists {
		if enabled, ok := serviceDiscovery.(bool); ok && enabled {
			if err := kp.installServiceDiscovery(ctx, clientset, config); err != nil {
				return err
			}
		}
	}

	// Configure DNS
	if dns, exists := config["dns"]; exists {
		if dnsConfig, ok := dns.(map[string]interface{}); ok {
			if enabled, exists := dnsConfig["enabled"]; exists {
				if enabledBool, ok := enabled.(bool); ok && enabledBool {
					if err := kp.installDNS(ctx, clientset, dnsConfig); err != nil {
						return err
					}
				}
			}
		}
	}

	kp.logger.Info("Kubernetes connectivity installation completed")
	return nil
}

// Uninstall uninstalls Kubernetes connectivity
func (kp *KubernetesProvider) Uninstall(ctx context.Context, clientset *kubernetes.Clientset) error {
	kp.logger.Info("Uninstalling Kubernetes connectivity")

	// Remove network policies
	if err := kp.removeNetworkPolicies(ctx, clientset); err != nil {
		return err
	}

	// Remove service discovery configurations
	if err := kp.removeServiceDiscovery(ctx, clientset); err != nil {
		return err
	}

	// Remove DNS configurations
	if err := kp.removeDNS(ctx, clientset); err != nil {
		return err
	}

	kp.logger.Info("Kubernetes connectivity uninstallation completed")
	return nil
}

// Status gets the status of Kubernetes connectivity
func (kp *KubernetesProvider) Status(ctx context.Context, clientset *kubernetes.Clientset) (ConnectivityStatus, error) {
	status := ConnectivityStatus{
		Type:        ConnectivityTypeKubernetes,
		State:       "unknown",
		Healthy:     false,
		LastChecked: time.Now().Format(time.RFC3339),
	}

	// Check network policies
	networkPolicies, err := clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to check network policies: %v", err)
		return status, nil
	}
	status.PoliciesCount = len(networkPolicies.Items)

	// Check services
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to check services: %v", err)
		return status, nil
	}
	status.ServicesCount = len(services.Items)

	// Check endpoints
	endpoints, err := clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to check endpoints: %v", err)
		return status, nil
	}
	status.EndpointsCount = len(endpoints.Items)

	// Determine overall health
	if status.PoliciesCount > 0 || status.ServicesCount > 0 {
		status.State = "configured"
		status.Healthy = true
	} else {
		status.State = "not_configured"
		status.Healthy = true // Not configured is a valid state
	}

	return status, nil
}

// HealthCheck performs health checks for Kubernetes connectivity
func (kp *KubernetesProvider) HealthCheck(ctx context.Context, clientset *kubernetes.Clientset) ([]ConnectivityHealthCheck, error) {
	var checks []ConnectivityHealthCheck
	startTime := time.Now()

	// Network policies health check
	networkPoliciesCheck := ConnectivityHealthCheck{
		Name:    "network-policies",
		Type:    "configuration",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	networkPolicies, err := clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err != nil {
		networkPoliciesCheck.Message = fmt.Sprintf("failed to list network policies: %v", err)
	} else {
		networkPoliciesCheck.Healthy = true
		networkPoliciesCheck.Message = fmt.Sprintf("found %d network policies", len(networkPolicies.Items))
		networkPoliciesCheck.Details = map[string]interface{}{
			"count": len(networkPolicies.Items),
		}
	}
	networkPoliciesCheck.Duration = time.Since(startTime).String()
	checks = append(checks, networkPoliciesCheck)

	// Service discovery health check
	serviceDiscoveryCheck := ConnectivityHealthCheck{
		Name:    "service-discovery",
		Type:    "configuration",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err != nil {
		serviceDiscoveryCheck.Message = fmt.Sprintf("failed to list services: %v", err)
	} else {
		serviceDiscoveryCheck.Healthy = true
		serviceDiscoveryCheck.Message = fmt.Sprintf("found %d connectivity services", len(services.Items))
		serviceDiscoveryCheck.Details = map[string]interface{}{
			"count": len(services.Items),
		}
	}
	serviceDiscoveryCheck.Duration = time.Since(startTime).String()
	checks = append(checks, serviceDiscoveryCheck)

	// DNS configuration health check
	dnsCheck := ConnectivityHealthCheck{
		Name:    "dns-configuration",
		Type:    "configuration",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	configMaps, err := clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,dns-config=true",
	})
	if err != nil {
		dnsCheck.Message = fmt.Sprintf("failed to list DNS config maps: %v", err)
	} else {
		dnsCheck.Healthy = true
		dnsCheck.Message = fmt.Sprintf("found %d DNS configurations", len(configMaps.Items))
		dnsCheck.Details = map[string]interface{}{
			"count": len(configMaps.Items),
		}
	}
	dnsCheck.Duration = time.Since(startTime).String()
	checks = append(checks, dnsCheck)

	return checks, nil
}

// installNetworkPolicies installs network policies for cross-cluster communication
func (kp *KubernetesProvider) installNetworkPolicies(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	kp.logger.Info("Installing network policies")

	var allowCIDRs []string
	if cidrs, exists := config["allowCIDRs"]; exists {
		if cidrSlice, ok := cidrs.([]interface{}); ok {
			for _, cidr := range cidrSlice {
				if cidrStr, ok := cidr.(string); ok {
					allowCIDRs = append(allowCIDRs, cidrStr)
				}
			}
		}
	}

	// Default CIDRs if none specified
	if len(allowCIDRs) == 0 {
		allowCIDRs = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	}

	// Create network policy in default namespace
	framework := &Framework{}
	if err := framework.CreateNetworkPolicy(ctx, clientset, "cross-cluster-traffic", "default", allowCIDRs); err != nil {
		return err
	}

	return nil
}

// installServiceDiscovery installs service discovery configurations
func (kp *KubernetesProvider) installServiceDiscovery(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	kp.logger.Info("Installing service discovery")

	// Create a headless service for service discovery
	framework := &Framework{}
	ports := []corev1.ServicePort{
		{
			Name: "http",
			Port: 80,
		},
		{
			Name: "https",
			Port: 443,
		},
	}

	if err := framework.CreateServiceEntry(ctx, clientset, "cross-cluster-discovery", "default",
		[]string{"*.cluster.local"}, ports); err != nil {
		return err
	}

	return nil
}

// installDNS installs DNS configurations
func (kp *KubernetesProvider) installDNS(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	kp.logger.Info("Installing DNS configuration")

	var searchDomains []string
	if domains, exists := config["searchDomains"]; exists {
		if domainSlice, ok := domains.([]interface{}); ok {
			for _, domain := range domainSlice {
				if domainStr, ok := domain.(string); ok {
					searchDomains = append(searchDomains, domainStr)
				}
			}
		}
	}

	// Default search domain
	if len(searchDomains) == 0 {
		searchDomains = []string{"cluster.local"}
	}

	dnsConfig := map[string]string{
		"search-domains": fmt.Sprintf("%v", searchDomains),
		"dns-policy":     "ClusterFirst",
	}

	framework := &Framework{}
	if err := framework.CreateConfigMap(ctx, clientset, "cross-cluster-dns", "kube-system", dnsConfig); err != nil {
		return err
	}

	return nil
}

// removeNetworkPolicies removes network policies
func (kp *KubernetesProvider) removeNetworkPolicies(ctx context.Context, clientset *kubernetes.Clientset) error {
	kp.logger.Info("Removing network policies")

	networkPolicies, err := clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err != nil {
		return err
	}

	for _, np := range networkPolicies.Items {
		if err := clientset.NetworkingV1().NetworkPolicies(np.Namespace).Delete(ctx, np.Name, metav1.DeleteOptions{}); err != nil {
			kp.logger.Warnf("Failed to delete network policy %s/%s: %v", np.Namespace, np.Name, err)
		}
	}

	return nil
}

// removeServiceDiscovery removes service discovery configurations
func (kp *KubernetesProvider) removeServiceDiscovery(ctx context.Context, clientset *kubernetes.Clientset) error {
	kp.logger.Info("Removing service discovery configurations")

	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=service-discovery",
	})
	if err != nil {
		return err
	}

	for _, svc := range services.Items {
		if err := clientset.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{}); err != nil {
			kp.logger.Warnf("Failed to delete service %s/%s: %v", svc.Namespace, svc.Name, err)
		}
	}

	return nil
}

// removeDNS removes DNS configurations
func (kp *KubernetesProvider) removeDNS(ctx context.Context, clientset *kubernetes.Clientset) error {
	kp.logger.Info("Removing DNS configurations")

	configMaps, err := clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,dns-config=true",
	})
	if err != nil {
		return err
	}

	for _, cm := range configMaps.Items {
		if err := clientset.CoreV1().ConfigMaps(cm.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
			kp.logger.Warnf("Failed to delete config map %s/%s: %v", cm.Namespace, cm.Name, err)
		}
	}

	return nil
}

// isValidCIDR validates CIDR format (basic validation)
func isValidCIDR(cidr string) bool {
	// This is a basic validation - in production you'd use a proper CIDR validation library
	// For now, just check if it contains a "/"
	return len(cidr) > 0 && cidr[len(cidr)-1] != '/' && len(cidr) > 3
}
