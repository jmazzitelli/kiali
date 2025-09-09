package connectivity

import (
	"context"
	"fmt"
	"time"

	"github.com/kiali/kiali-integration-framework/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// ManualProvider implements manual connectivity configuration
type ManualProvider struct {
	logger *utils.Logger
}

// NewManualProvider creates a new manual connectivity provider
func NewManualProvider() *ManualProvider {
	return &ManualProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Type returns the connectivity type
func (mp *ManualProvider) Type() ConnectivityType {
	return ConnectivityTypeManual
}

// ValidateConfig validates the manual connectivity configuration
func (mp *ManualProvider) ValidateConfig(config map[string]interface{}) error {
	if config == nil {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"Manual connectivity configuration is required", nil)
	}

	// Check for custom configuration
	if custom, exists := config["custom"]; exists {
		if customConfig, ok := custom.(map[string]interface{}); ok {
			if enabled, exists := customConfig["enabled"]; exists {
				if enabledBool, ok := enabled.(bool); ok && enabledBool {
					// Validate that configurations are provided
					if configs, exists := customConfig["configurations"]; exists {
						if configSlice, ok := configs.([]interface{}); ok {
							if len(configSlice) == 0 {
								return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
									"custom configurations cannot be empty when custom connectivity is enabled", nil)
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// Install installs manual connectivity
func (mp *ManualProvider) Install(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	mp.logger.Info("Installing manual connectivity")

	// Apply custom configurations
	if custom, exists := config["custom"]; exists {
		if customConfig, ok := custom.(map[string]interface{}); ok {
			if err := mp.installCustomConfigurations(ctx, clientset, customConfig); err != nil {
				return err
			}
		}
	}

	// Apply user-provided resources
	if resources, exists := config["resources"]; exists {
		if resourceList, ok := resources.([]interface{}); ok {
			if err := mp.installResources(ctx, clientset, resourceList); err != nil {
				return err
			}
		}
	}

	mp.logger.Info("Manual connectivity installation completed")
	return nil
}

// Uninstall uninstalls manual connectivity
func (mp *ManualProvider) Uninstall(ctx context.Context, clientset *kubernetes.Clientset) error {
	mp.logger.Info("Uninstalling manual connectivity")

	// Remove custom configurations
	if err := mp.removeCustomConfigurations(ctx, clientset); err != nil {
		return err
	}

	// Remove user-provided resources
	if err := mp.removeResources(ctx, clientset); err != nil {
		return err
	}

	mp.logger.Info("Manual connectivity uninstallation completed")
	return nil
}

// Status gets the status of manual connectivity
func (mp *ManualProvider) Status(ctx context.Context, clientset *kubernetes.Clientset) (ConnectivityStatus, error) {
	status := ConnectivityStatus{
		Type:        ConnectivityTypeManual,
		State:       "unknown",
		Healthy:     false,
		LastChecked: time.Now().Format(time.RFC3339),
	}

	// Check for manual connectivity configurations
	configMaps, err := clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=manual",
	})
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to check manual configs: %v", err)
		return status, nil
	}

	status.PoliciesCount = len(configMaps.Items)

	// Check for user-provided services
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=manual",
	})
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to check manual services: %v", err)
		return status, nil
	}

	status.ServicesCount = len(services.Items)

	// Check for user-provided endpoints
	endpoints, err := clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=manual",
	})
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to check manual endpoints: %v", err)
		return status, nil
	}

	status.EndpointsCount = len(endpoints.Items)

	// Determine overall health
	if status.PoliciesCount > 0 || status.ServicesCount > 0 || status.EndpointsCount > 0 {
		status.State = "configured"
		status.Healthy = true
	} else {
		status.State = "not_configured"
		status.Healthy = true // Not configured is a valid state
	}

	return status, nil
}

// HealthCheck performs health checks for manual connectivity
func (mp *ManualProvider) HealthCheck(ctx context.Context, clientset *kubernetes.Clientset) ([]ConnectivityHealthCheck, error) {
	var checks []ConnectivityHealthCheck
	startTime := time.Now()

	// Manual configuration check
	manualConfigCheck := ConnectivityHealthCheck{
		Name:    "manual-configuration",
		Type:    "configuration",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	configMaps, err := clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=manual",
	})
	if err != nil {
		manualConfigCheck.Message = fmt.Sprintf("failed to check manual configurations: %v", err)
	} else {
		manualConfigCheck.Healthy = true
		manualConfigCheck.Message = fmt.Sprintf("found %d manual configurations", len(configMaps.Items))
		manualConfigCheck.Details = map[string]interface{}{
			"configCount": len(configMaps.Items),
		}
	}
	manualConfigCheck.Duration = time.Since(startTime).String()
	checks = append(checks, manualConfigCheck)

	// Manual services check
	servicesCheck := ConnectivityHealthCheck{
		Name:    "manual-services",
		Type:    "configuration",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=manual",
	})
	if err != nil {
		servicesCheck.Message = fmt.Sprintf("failed to check manual services: %v", err)
	} else {
		servicesCheck.Healthy = true
		servicesCheck.Message = fmt.Sprintf("found %d manual services", len(services.Items))
		servicesCheck.Details = map[string]interface{}{
			"serviceCount": len(services.Items),
		}
	}
	servicesCheck.Duration = time.Since(startTime).String()
	checks = append(checks, servicesCheck)

	// Manual endpoints check
	endpointsCheck := ConnectivityHealthCheck{
		Name:    "manual-endpoints",
		Type:    "configuration",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	endpoints, err := clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=manual",
	})
	if err != nil {
		endpointsCheck.Message = fmt.Sprintf("failed to check manual endpoints: %v", err)
	} else {
		endpointsCheck.Healthy = true
		endpointsCheck.Message = fmt.Sprintf("found %d manual endpoints", len(endpoints.Items))
		endpointsCheck.Details = map[string]interface{}{
			"endpointCount": len(endpoints.Items),
		}
	}
	endpointsCheck.Duration = time.Since(startTime).String()
	checks = append(checks, endpointsCheck)

	return checks, nil
}

// installCustomConfigurations installs custom connectivity configurations
func (mp *ManualProvider) installCustomConfigurations(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	mp.logger.Info("Installing custom connectivity configurations")

	framework := &Framework{}

	// Process configurations
	if configurations, exists := config["configurations"]; exists {
		if configList, ok := configurations.([]interface{}); ok {
			for i, cfg := range configList {
				if configMap, ok := cfg.(map[string]interface{}); ok {
					configName := fmt.Sprintf("manual-config-%d", i+1)
					if name, exists := configMap["name"]; exists {
						if nameStr, ok := name.(string); ok {
							configName = nameStr
						}
					}

					// Convert config to string map
					stringConfig := make(map[string]string)
					for k, v := range configMap {
						if k != "name" {
							if vStr, ok := v.(string); ok {
								stringConfig[k] = vStr
							} else {
								stringConfig[k] = fmt.Sprintf("%v", v)
							}
						}
					}

					if err := framework.CreateConfigMap(ctx, clientset, configName, "default", stringConfig); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// installResources installs user-provided Kubernetes resources
func (mp *ManualProvider) installResources(ctx context.Context, clientset *kubernetes.Clientset, resources []interface{}) error {
	mp.logger.Info("Installing user-provided resources")

	framework := &Framework{}

	for _, resource := range resources {
		if resourceMap, ok := resource.(map[string]interface{}); ok {
			resourceType, hasType := resourceMap["type"]
			resourceName, hasName := resourceMap["name"]
			resourceNamespace, hasNamespace := resourceMap["namespace"]

			if !hasType || !hasName {
				continue
			}

			namespace := "default"
			if hasNamespace {
				if nsStr, ok := resourceNamespace.(string); ok {
					namespace = nsStr
				}
			}

			switch resourceType {
			case "service":
				if ports, exists := resourceMap["ports"]; exists {
					if portList, ok := ports.([]interface{}); ok {
						var servicePorts []corev1.ServicePort
						for _, port := range portList {
							if portMap, ok := port.(map[string]interface{}); ok {
								servicePort := corev1.ServicePort{}
								if name, exists := portMap["name"]; exists {
									if nameStr, ok := name.(string); ok {
										servicePort.Name = nameStr
									}
								}
								if portNum, exists := portMap["port"]; exists {
									if portInt, ok := portNum.(int); ok {
										servicePort.Port = int32(portInt)
									}
								}
								if targetPort, exists := portMap["targetPort"]; exists {
									if tpInt, ok := targetPort.(int); ok {
										servicePort.TargetPort = intstr.FromInt(tpInt)
									}
								}
								servicePorts = append(servicePorts, servicePort)
							}
						}

						var hosts []string
						if hostList, exists := resourceMap["hosts"]; exists {
							if hList, ok := hostList.([]interface{}); ok {
								for _, host := range hList {
									if hostStr, ok := host.(string); ok {
										hosts = append(hosts, hostStr)
									}
								}
							}
						}

						if err := framework.CreateServiceEntry(ctx, clientset, resourceName.(string), namespace, hosts, servicePorts); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

// removeCustomConfigurations removes custom connectivity configurations
func (mp *ManualProvider) removeCustomConfigurations(ctx context.Context, clientset *kubernetes.Clientset) error {
	mp.logger.Info("Removing custom connectivity configurations")

	configMaps, err := clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=manual",
	})
	if err != nil {
		return err
	}

	for _, cm := range configMaps.Items {
		if err := clientset.CoreV1().ConfigMaps(cm.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
			mp.logger.Warnf("Failed to delete config map %s/%s: %v", cm.Namespace, cm.Name, err)
		}
	}

	return nil
}

// removeResources removes user-provided resources
func (mp *ManualProvider) removeResources(ctx context.Context, clientset *kubernetes.Clientset) error {
	mp.logger.Info("Removing user-provided resources")

	// Remove services
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=manual",
	})
	if err == nil {
		for _, svc := range services.Items {
			if err := clientset.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{}); err != nil {
				mp.logger.Warnf("Failed to delete service %s/%s: %v", svc.Namespace, svc.Name, err)
			}
		}
	}

	// Remove endpoints
	endpoints, err := clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,connectivity-type=manual",
	})
	if err == nil {
		for _, ep := range endpoints.Items {
			if err := clientset.CoreV1().Endpoints(ep.Namespace).Delete(ctx, ep.Name, metav1.DeleteOptions{}); err != nil {
				mp.logger.Warnf("Failed to delete endpoints %s/%s: %v", ep.Namespace, ep.Name, err)
			}
		}
	}

	return nil
}
