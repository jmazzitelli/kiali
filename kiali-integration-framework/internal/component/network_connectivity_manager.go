package component

import (
	"context"
	"fmt"

	"github.com/kiali/kiali-integration-framework/pkg/connectivity"
	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NetworkConnectivityManager implements the ComponentManagerInterface for network connectivity
type NetworkConnectivityManager struct {
	logger    *utils.Logger
	framework *connectivity.Framework
}

// NewNetworkConnectivityManager creates a new Network Connectivity component manager
func NewNetworkConnectivityManager() *NetworkConnectivityManager {
	framework := connectivity.NewFramework()

	// Register connectivity providers
	framework.RegisterProvider(connectivity.NewKubernetesProvider())
	framework.RegisterProvider(connectivity.NewIstioProvider())
	framework.RegisterProvider(connectivity.NewLinkerdProvider())
	framework.RegisterProvider(connectivity.NewManualProvider())

	// Initialize default templates
	framework.InitializeDefaultTemplates()

	// Initialize service discovery providers
	framework.InitializeServiceDiscoveryProviders()

	return &NetworkConnectivityManager{
		logger:    utils.GetGlobalLogger(),
		framework: framework,
	}
}

// Name returns the name of the manager
func (ncm *NetworkConnectivityManager) Name() string {
	return "Network Connectivity Manager"
}

// Type returns the component type this manager handles
func (ncm *NetworkConnectivityManager) Type() types.ComponentType {
	return types.ComponentTypeNetworkConnectivity
}

// ValidateConfig validates the Network Connectivity component configuration
func (ncm *NetworkConnectivityManager) ValidateConfig(config types.ComponentConfig) error {
	if config.Type != types.ComponentTypeNetworkConnectivity {
		return utils.NewFrameworkError(utils.ErrCodeInvalidParameter,
			"invalid component type for Network Connectivity manager", nil)
	}

	if config.Version == "" {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"Network Connectivity version is required", nil)
	}

	// Validate network connectivity-specific configuration
	connectivityConfig, exists := config.Config["connectivity"]
	if !exists {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"connectivity configuration is required", nil)
	}

	connectivityMap, ok := connectivityConfig.(map[string]interface{})
	if !ok {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"connectivity configuration must be a map", nil)
	}

	// Validate connectivity type if specified
	if connectivityType, exists := connectivityMap["type"]; exists {
		var ct string
		if connectivityTypeStr, ok := connectivityType.(string); !ok || connectivityTypeStr == "" {
			return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
				"connectivity type must be a non-empty string", nil)
		} else {
			ct = connectivityTypeStr
		}

		validTypes := []string{"kubernetes", "istio", "linkerd", "manual"}
		valid := false
		for _, vt := range validTypes {
			if ct == vt {
				valid = true
				break
			}
		}
		if !valid {
			return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
				fmt.Sprintf("connectivity type must be one of: %v", validTypes), nil)
		}
	}

	return nil
}

// Install installs Network Connectivity in the cluster
func (ncm *NetworkConnectivityManager) Install(ctx context.Context, env *types.Environment, config types.ComponentConfig) error {
	if err := ncm.ValidateConfig(config); err != nil {
		return err
	}

	return ncm.logger.LogOperationWithContext("install network connectivity", map[string]interface{}{
		"version": config.Version,
		"config":  config.Config,
	}, func() error {
		ncm.logger.Infof("Installing Network Connectivity version %s", config.Version)

		// Get Kubernetes client
		clientset, err := ncm.getKubernetesClient(ctx, env)
		if err != nil {
			return utils.WrapError(err, utils.ErrCodeClusterUnhealthy,
				"failed to get Kubernetes client")
		}

		// Check if connectivity is already configured
		configured, err := ncm.isConnectivityConfigured(ctx, clientset)
		if err != nil {
			return utils.WrapError(err, utils.ErrCodeInternalError,
				"failed to check connectivity configuration status")
		}

		if configured {
			ncm.logger.Warn("Network connectivity is already configured, skipping installation")
			return nil
		}

		// Install network connectivity
		if err := ncm.installConnectivity(ctx, env, config, clientset); err != nil {
			return utils.WrapError(err, utils.ErrCodeComponentInstallFailed,
				"failed to install network connectivity")
		}

		ncm.logger.Info("Network connectivity installation completed successfully")
		return nil
	})
}

// Uninstall uninstalls Network Connectivity from the cluster
func (ncm *NetworkConnectivityManager) Uninstall(ctx context.Context, env *types.Environment, name string) error {
	return ncm.logger.LogOperation("uninstall network connectivity", func() error {
		ncm.logger.Info("Uninstalling Network Connectivity")

		// Get Kubernetes client
		clientset, err := ncm.getKubernetesClient(ctx, env)
		if err != nil {
			return utils.WrapError(err, utils.ErrCodeClusterUnhealthy,
				"failed to get Kubernetes client")
		}

		// Check if connectivity is configured
		configured, err := ncm.isConnectivityConfigured(ctx, clientset)
		if err != nil {
			return utils.WrapError(err, utils.ErrCodeInternalError,
				"failed to check connectivity configuration status")
		}

		if !configured {
			ncm.logger.Warn("Network connectivity is not configured, nothing to uninstall")
			return nil
		}

		// Uninstall network connectivity
		if err := ncm.uninstallConnectivity(ctx, env, clientset); err != nil {
			return utils.WrapError(err, utils.ErrCodeComponentUninstallFailed,
				"failed to uninstall network connectivity")
		}

		ncm.logger.Info("Network connectivity uninstallation completed successfully")
		return nil
	})
}

// GetStatus gets the current status of Network Connectivity
func (ncm *NetworkConnectivityManager) GetStatus(ctx context.Context, env *types.Environment, component *types.Component) (types.ComponentStatus, error) {
	clientset, err := ncm.getKubernetesClient(ctx, env)
	if err != nil {
		return types.ComponentStatusFailed, utils.WrapError(err, utils.ErrCodeClusterUnhealthy,
			"failed to get Kubernetes client")
	}

	configured, err := ncm.isConnectivityConfigured(ctx, clientset)
	if err != nil {
		return types.ComponentStatusFailed, utils.WrapError(err, utils.ErrCodeInternalError,
			"failed to check connectivity status")
	}

	if !configured {
		return types.ComponentStatusNotInstalled, nil
	}

	// Check if connectivity is healthy
	healthy, err := ncm.isConnectivityHealthy(ctx, clientset)
	if err != nil {
		return types.ComponentStatusFailed, utils.WrapError(err, utils.ErrCodeInternalError,
			"failed to check connectivity health")
	}

	if healthy {
		return types.ComponentStatusInstalled, nil
	}

	return types.ComponentStatusFailed, nil
}

// Update updates Network Connectivity to a new version or configuration
func (ncm *NetworkConnectivityManager) Update(ctx context.Context, env *types.Environment, config types.ComponentConfig) error {
	if err := ncm.ValidateConfig(config); err != nil {
		return err
	}

	return ncm.logger.LogOperation("update network connectivity", func() error {
		ncm.logger.Infof("Updating Network Connectivity to version %s", config.Version)

		// For now, implement as uninstall + install
		// TODO: Implement proper in-place updates when supported
		if err := ncm.Uninstall(ctx, env, "network-connectivity"); err != nil {
			return utils.WrapError(err, utils.ErrCodeComponentUpdateFailed,
				"failed to uninstall old connectivity configuration")
		}

		if err := ncm.Install(ctx, env, config); err != nil {
			return utils.WrapError(err, utils.ErrCodeComponentUpdateFailed,
				"failed to install new connectivity configuration")
		}

		ncm.logger.Info("Network connectivity update completed successfully")
		return nil
	})
}

// getKubernetesClient creates a Kubernetes client from the environment
func (ncm *NetworkConnectivityManager) getKubernetesClient(ctx context.Context, env *types.Environment) (*kubernetes.Clientset, error) {
	// TODO: Implement proper kubeconfig handling from environment
	// For now, use in-cluster config or default kubeconfig
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to default kubeconfig
		config, err = utils.GetKubeConfig()
		if err != nil {
			return nil, err
		}
	}

	return kubernetes.NewForConfig(config)
}

// isConnectivityConfigured checks if network connectivity is already configured in the cluster
func (ncm *NetworkConnectivityManager) isConnectivityConfigured(ctx context.Context, clientset *kubernetes.Clientset) (bool, error) {
	// Check for connectivity-related resources
	// This is a simplified check - in reality, we'd check for specific connectivity configurations

	// Check for network policies that might indicate connectivity setup
	networkPolicies, err := clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	if err == nil && len(networkPolicies.Items) > 0 {
		return true, nil
	}

	// Check for services that might be part of connectivity setup
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=connectivity",
	})
	if err == nil && len(services.Items) > 0 {
		return true, nil
	}

	// Check for configmaps related to connectivity
	configMaps, err := clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=connectivity",
	})
	if err == nil && len(configMaps.Items) > 0 {
		return true, nil
	}

	return false, nil
}

// isConnectivityHealthy checks if network connectivity is running and healthy
func (ncm *NetworkConnectivityManager) isConnectivityHealthy(ctx context.Context, clientset *kubernetes.Clientset) (bool, error) {
	// Check for connectivity-related deployments
	deployments, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=connectivity",
	})
	if err != nil {
		return false, err
	}

	// If we have connectivity deployments, check if they're ready
	for _, deployment := range deployments.Items {
		if deployment.Status.ReadyReplicas != deployment.Status.Replicas {
			return false, nil
		}
	}

	// If no specific connectivity deployments found, assume basic connectivity is working
	return true, nil
}

// installConnectivity performs the actual network connectivity installation
func (ncm *NetworkConnectivityManager) installConnectivity(ctx context.Context, env *types.Environment, config types.ComponentConfig, clientset *kubernetes.Clientset) error {
	ncm.logger.Info("Installing network connectivity using framework")

	connectivityConfig := config.Config["connectivity"].(map[string]interface{})

	// Check if a template is specified
	if templateName, exists := connectivityConfig["template"]; exists {
		if templateStr, ok := templateName.(string); ok {
			ncm.logger.Infof("Installing connectivity using template: %s", templateStr)

			// Merge template config with provided config
			templateConfig := make(map[string]interface{})
			for k, v := range connectivityConfig {
				if k != "template" {
					templateConfig[k] = v
				}
			}

			return ncm.framework.InstallConnectivity(ctx, clientset, templateStr, templateConfig)
		}
	}

	// Manual configuration without template
	connectivityType := "kubernetes" // default
	if ct, exists := connectivityConfig["type"]; exists {
		connectivityType = ct.(string)
	}

	ncm.logger.Infof("Installing connectivity type: %s", connectivityType)

	var connType connectivity.ConnectivityType
	switch connectivityType {
	case "kubernetes":
		connType = connectivity.ConnectivityTypeKubernetes
	case "istio":
		connType = connectivity.ConnectivityTypeIstio
	case "linkerd":
		connType = connectivity.ConnectivityTypeLinkerd
	case "manual":
		connType = connectivity.ConnectivityTypeManual
	default:
		return fmt.Errorf("unsupported connectivity type: %s", connectivityType)
	}

	// Get the provider
	provider, err := ncm.framework.GetProvider(connType)
	if err != nil {
		return fmt.Errorf("failed to get connectivity provider: %w", err)
	}

	// Install using the provider
	if err := provider.Install(ctx, clientset, connectivityConfig); err != nil {
		return fmt.Errorf("failed to install connectivity: %w", err)
	}

	ncm.logger.Info("Network connectivity installation completed")
	return nil
}

// uninstallConnectivity performs the actual network connectivity uninstallation
func (ncm *NetworkConnectivityManager) uninstallConnectivity(ctx context.Context, env *types.Environment, clientset *kubernetes.Clientset) error {
	ncm.logger.Info("Uninstalling network connectivity using framework")

	// For now, we'll remove all connectivity configurations
	// In the future, we could track what was installed and remove selectively

	// Remove network policies
	networkPolicies, err := clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err == nil {
		for _, np := range networkPolicies.Items {
			if err := clientset.NetworkingV1().NetworkPolicies(np.Namespace).Delete(ctx, np.Name, metav1.DeleteOptions{}); err != nil {
				ncm.logger.Warnf("Failed to delete network policy %s/%s: %v", np.Namespace, np.Name, err)
			}
		}
	}

	// Remove services
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err == nil {
		for _, svc := range services.Items {
			if err := clientset.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{}); err != nil {
				ncm.logger.Warnf("Failed to delete service %s/%s: %v", svc.Namespace, svc.Name, err)
			}
		}
	}

	// Remove config maps
	configMaps, err := clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err == nil {
		for _, cm := range configMaps.Items {
			if err := clientset.CoreV1().ConfigMaps(cm.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
				ncm.logger.Warnf("Failed to delete config map %s/%s: %v", cm.Namespace, cm.Name, err)
			}
		}
	}

	ncm.logger.Info("Network connectivity uninstallation completed")
	return nil
}

// GetFramework returns the connectivity framework for advanced operations
func (ncm *NetworkConnectivityManager) GetFramework() *connectivity.Framework {
	return ncm.framework
}

// GetConnectivityTemplates returns available connectivity templates
func (ncm *NetworkConnectivityManager) GetConnectivityTemplates() map[string]*connectivity.ConnectivityTemplate {
	return ncm.framework.ListTemplates()
}

// GetConnectivityStatus returns the status of connectivity for a specific type
func (ncm *NetworkConnectivityManager) GetConnectivityStatus(ctx context.Context, clientset *kubernetes.Clientset, connectivityType connectivity.ConnectivityType) (connectivity.ConnectivityStatus, error) {
	return ncm.framework.GetConnectivityStatus(ctx, clientset, connectivityType)
}

// RunConnectivityHealthChecks runs health checks for a specific connectivity type
func (ncm *NetworkConnectivityManager) RunConnectivityHealthChecks(ctx context.Context, clientset *kubernetes.Clientset, connectivityType connectivity.ConnectivityType) ([]connectivity.ConnectivityHealthCheck, error) {
	return ncm.framework.RunHealthChecks(ctx, clientset, connectivityType)
}
