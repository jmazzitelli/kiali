package connectivity

import (
	"context"
	"fmt"
	"time"

	"github.com/kiali/kiali-integration-framework/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// IstioProvider implements Istio-based connectivity
type IstioProvider struct {
	logger *utils.Logger
}

// NewIstioProvider creates a new Istio connectivity provider
func NewIstioProvider() *IstioProvider {
	return &IstioProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Type returns the connectivity type
func (ip *IstioProvider) Type() ConnectivityType {
	return ConnectivityTypeIstio
}

// ValidateConfig validates the Istio connectivity configuration
func (ip *IstioProvider) ValidateConfig(config map[string]interface{}) error {
	if config == nil {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"Istio connectivity configuration is required", nil)
	}

	// Check for service mesh configuration
	if serviceMesh, exists := config["serviceMesh"]; exists {
		if meshConfig, ok := serviceMesh.(map[string]interface{}); ok {
			if enabled, exists := meshConfig["enabled"]; exists {
				if enabledBool, ok := enabled.(bool); ok && enabledBool {
					// Validate discovery selectors if provided
					if selectors, exists := meshConfig["discoverySelectors"]; exists {
						if selectorSlice, ok := selectors.([]interface{}); ok {
							for _, selector := range selectorSlice {
								if selectorMap, ok := selector.(map[string]interface{}); ok {
									if len(selectorMap) == 0 {
										return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
											"discovery selectors cannot be empty", nil)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// Install installs Istio connectivity
func (ip *IstioProvider) Install(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	ip.logger.Info("Installing Istio connectivity")

	// Configure service mesh
	if serviceMesh, exists := config["serviceMesh"]; exists {
		if meshConfig, ok := serviceMesh.(map[string]interface{}); ok {
			if err := ip.installServiceMesh(ctx, clientset, meshConfig); err != nil {
				return err
			}
		}
	}

	// Configure traffic management
	if trafficManagement, exists := config["trafficManagement"]; exists {
		if tmConfig, ok := trafficManagement.(map[string]interface{}); ok {
			if err := ip.installTrafficManagement(ctx, clientset, tmConfig); err != nil {
				return err
			}
		}
	}

	ip.logger.Info("Istio connectivity installation completed")
	return nil
}

// Uninstall uninstalls Istio connectivity
func (ip *IstioProvider) Uninstall(ctx context.Context, clientset *kubernetes.Clientset) error {
	ip.logger.Info("Uninstalling Istio connectivity")

	// Remove traffic management configurations
	if err := ip.removeTrafficManagement(ctx, clientset); err != nil {
		return err
	}

	// Remove service mesh configurations
	if err := ip.removeServiceMesh(ctx, clientset); err != nil {
		return err
	}

	ip.logger.Info("Istio connectivity uninstallation completed")
	return nil
}

// Status gets the status of Istio connectivity
func (ip *IstioProvider) Status(ctx context.Context, clientset *kubernetes.Clientset) (ConnectivityStatus, error) {
	status := ConnectivityStatus{
		Type:        ConnectivityTypeIstio,
		State:       "unknown",
		Healthy:     false,
		LastChecked: time.Now().Format(time.RFC3339),
	}

	// Check for Istio configurations
	configMaps, err := clientset.CoreV1().ConfigMaps("istio-system").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err != nil {
		// If istio-system doesn't exist or we can't access it, that's okay
		if !isNotFoundError(err) {
			status.ErrorMessage = fmt.Sprintf("failed to check Istio configs: %v", err)
			return status, nil
		}
	}

	// Check for Istio-related services
	services, err := clientset.CoreV1().Services("istio-system").List(ctx, metav1.ListOptions{
		LabelSelector: "istio.io/rev",
	})
	if err != nil {
		if !isNotFoundError(err) {
			status.ErrorMessage = fmt.Sprintf("failed to check Istio services: %v", err)
			return status, nil
		}
	}

	status.ServicesCount = len(services.Items)
	status.PoliciesCount = len(configMaps.Items)

	// Check if Istio is installed
	if status.ServicesCount > 0 {
		status.State = "istio_installed"
		status.Healthy = true
	} else {
		status.State = "istio_not_found"
		status.ErrorMessage = "Istio does not appear to be installed in the cluster"
	}

	return status, nil
}

// HealthCheck performs health checks for Istio connectivity
func (ip *IstioProvider) HealthCheck(ctx context.Context, clientset *kubernetes.Clientset) ([]ConnectivityHealthCheck, error) {
	var checks []ConnectivityHealthCheck
	startTime := time.Now()

	// Istio installation check
	istioCheck := ConnectivityHealthCheck{
		Name:    "istio-installation",
		Type:    "istio",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	// Check if istio-system namespace exists
	_, err := clientset.CoreV1().Namespaces().Get(ctx, "istio-system", metav1.GetOptions{})
	if err != nil {
		if isNotFoundError(err) {
			istioCheck.Message = "istio-system namespace not found - Istio may not be installed"
		} else {
			istioCheck.Message = fmt.Sprintf("failed to check istio-system namespace: %v", err)
		}
	} else {
		istioCheck.Healthy = true
		istioCheck.Message = "istio-system namespace found"
	}
	istioCheck.Duration = time.Since(startTime).String()
	checks = append(checks, istioCheck)

	// Pilot health check
	pilotCheck := ConnectivityHealthCheck{
		Name:    "istio-pilot",
		Type:    "istio",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	if istioCheck.Healthy {
		pods, err := clientset.CoreV1().Pods("istio-system").List(ctx, metav1.ListOptions{
			LabelSelector: "istio=pilot",
		})
		if err != nil {
			pilotCheck.Message = fmt.Sprintf("failed to check pilot pods: %v", err)
		} else if len(pods.Items) == 0 {
			pilotCheck.Message = "no pilot pods found"
		} else {
			healthyPods := 0
			for _, pod := range pods.Items {
				if pod.Status.Phase == "Running" {
					healthyPods++
				}
			}
			if healthyPods > 0 {
				pilotCheck.Healthy = true
				pilotCheck.Message = fmt.Sprintf("%d/%d pilot pods running", healthyPods, len(pods.Items))
			} else {
				pilotCheck.Message = "no pilot pods are running"
			}
			pilotCheck.Details = map[string]interface{}{
				"totalPods":   len(pods.Items),
				"runningPods": healthyPods,
			}
		}
	} else {
		pilotCheck.Message = "skipped - Istio not installed"
	}
	pilotCheck.Duration = time.Since(startTime).String()
	checks = append(checks, pilotCheck)

	// Service mesh configuration check
	meshConfigCheck := ConnectivityHealthCheck{
		Name:    "service-mesh-config",
		Type:    "istio",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	if istioCheck.Healthy {
		configMaps, err := clientset.CoreV1().ConfigMaps("istio-system").List(ctx, metav1.ListOptions{
			LabelSelector: "connectivity-framework=true",
		})
		if err != nil {
			meshConfigCheck.Message = fmt.Sprintf("failed to check mesh configs: %v", err)
		} else {
			meshConfigCheck.Healthy = true
			meshConfigCheck.Message = fmt.Sprintf("found %d mesh configurations", len(configMaps.Items))
			meshConfigCheck.Details = map[string]interface{}{
				"configCount": len(configMaps.Items),
			}
		}
	} else {
		meshConfigCheck.Message = "skipped - Istio not installed"
	}
	meshConfigCheck.Duration = time.Since(startTime).String()
	checks = append(checks, meshConfigCheck)

	return checks, nil
}

// installServiceMesh installs Istio service mesh configurations
func (ip *IstioProvider) installServiceMesh(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	ip.logger.Info("Installing Istio service mesh configurations")

	framework := &Framework{}

	// Create discovery selectors configuration
	var discoverySelectors []string
	if selectors, exists := config["discoverySelectors"]; exists {
		if selectorSlice, ok := selectors.([]interface{}); ok {
			for _, selector := range selectorSlice {
				if selectorMap, ok := selector.(map[string]interface{}); ok {
					for key, value := range selectorMap {
						if valueStr, ok := value.(string); ok {
							discoverySelectors = append(discoverySelectors, fmt.Sprintf("%s=%s", key, valueStr))
						}
					}
				}
			}
		}
	}

	meshConfig := map[string]string{
		"discovery-selectors": fmt.Sprintf("%v", discoverySelectors),
		"mesh-config":         "enabled",
		"cross-cluster":       "enabled",
	}

	if err := framework.CreateConfigMap(ctx, clientset, "istio-mesh-config", "istio-system", meshConfig); err != nil {
		return err
	}

	return nil
}

// installTrafficManagement installs Istio traffic management configurations
func (ip *IstioProvider) installTrafficManagement(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	ip.logger.Info("Installing Istio traffic management configurations")

	framework := &Framework{}

	// Create traffic management configuration
	loadBalancing := "ROUND_ROBIN"
	if lb, exists := config["loadBalancing"]; exists {
		if lbStr, ok := lb.(string); ok {
			loadBalancing = lbStr
		}
	}

	trafficConfig := map[string]string{
		"load-balancing":    loadBalancing,
		"circuit-breaker":   fmt.Sprintf("%v", config["circuitBreaker"]),
		"traffic-policies":  "enabled",
		"destination-rules": "enabled",
		"virtual-services":  "enabled",
	}

	if err := framework.CreateConfigMap(ctx, clientset, "istio-traffic-config", "istio-system", trafficConfig); err != nil {
		return err
	}

	return nil
}

// removeServiceMesh removes Istio service mesh configurations
func (ip *IstioProvider) removeServiceMesh(ctx context.Context, clientset *kubernetes.Clientset) error {
	ip.logger.Info("Removing Istio service mesh configurations")

	configMaps, err := clientset.CoreV1().ConfigMaps("istio-system").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,istio-mesh-config=true",
	})
	if err != nil {
		if !isNotFoundError(err) {
			return err
		}
		return nil // istio-system doesn't exist, nothing to clean up
	}

	for _, cm := range configMaps.Items {
		if err := clientset.CoreV1().ConfigMaps(cm.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
			ip.logger.Warnf("Failed to delete config map %s/%s: %v", cm.Namespace, cm.Name, err)
		}
	}

	return nil
}

// removeTrafficManagement removes Istio traffic management configurations
func (ip *IstioProvider) removeTrafficManagement(ctx context.Context, clientset *kubernetes.Clientset) error {
	ip.logger.Info("Removing Istio traffic management configurations")

	configMaps, err := clientset.CoreV1().ConfigMaps("istio-system").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,istio-traffic-config=true",
	})
	if err != nil {
		if !isNotFoundError(err) {
			return err
		}
		return nil // istio-system doesn't exist, nothing to clean up
	}

	for _, cm := range configMaps.Items {
		if err := clientset.CoreV1().ConfigMaps(cm.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
			ip.logger.Warnf("Failed to delete config map %s/%s: %v", cm.Namespace, cm.Name, err)
		}
	}

	return nil
}

// isNotFoundError checks if an error is a "not found" error
func isNotFoundError(err error) bool {
	return err != nil && err.Error() != "" &&
		(len(err.Error()) >= 9 && err.Error()[:9] == "Not Found" ||
			len(err.Error()) >= 10 && err.Error()[:10] == "not found")
}
