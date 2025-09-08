package connectivity

import (
	"context"
	"fmt"
	"time"

	"github.com/kiali/kiali-integration-framework/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// LinkerdProvider implements Linkerd-based connectivity
type LinkerdProvider struct {
	logger *utils.Logger
}

// NewLinkerdProvider creates a new Linkerd connectivity provider
func NewLinkerdProvider() *LinkerdProvider {
	return &LinkerdProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Type returns the connectivity type
func (lp *LinkerdProvider) Type() ConnectivityType {
	return ConnectivityTypeLinkerd
}

// ValidateConfig validates the Linkerd connectivity configuration
func (lp *LinkerdProvider) ValidateConfig(config map[string]interface{}) error {
	if config == nil {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"Linkerd connectivity configuration is required", nil)
	}

	// Check for service mesh configuration
	if serviceMesh, exists := config["serviceMesh"]; exists {
		if meshConfig, ok := serviceMesh.(map[string]interface{}); ok {
			if enabled, exists := meshConfig["enabled"]; exists {
				if enabledBool, ok := enabled.(bool); ok && enabledBool {
					// Validate trust domain if provided
					if trustDomain, exists := meshConfig["trustDomain"]; exists {
						if tdStr, ok := trustDomain.(string); ok && tdStr == "" {
							return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
								"trust domain cannot be empty when service mesh is enabled", nil)
						}
					}
				}
			}
		}
	}

	return nil
}

// Install installs Linkerd connectivity
func (lp *LinkerdProvider) Install(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	lp.logger.Info("Installing Linkerd connectivity")

	// Configure service mesh
	if serviceMesh, exists := config["serviceMesh"]; exists {
		if meshConfig, ok := serviceMesh.(map[string]interface{}); ok {
			if err := lp.installServiceMesh(ctx, clientset, meshConfig); err != nil {
				return err
			}
		}
	}

	// Configure traffic management
	if trafficManagement, exists := config["trafficManagement"]; exists {
		if tmConfig, ok := trafficManagement.(map[string]interface{}); ok {
			if err := lp.installTrafficManagement(ctx, clientset, tmConfig); err != nil {
				return err
			}
		}
	}

	lp.logger.Info("Linkerd connectivity installation completed")
	return nil
}

// Uninstall uninstalls Linkerd connectivity
func (lp *LinkerdProvider) Uninstall(ctx context.Context, clientset *kubernetes.Clientset) error {
	lp.logger.Info("Uninstalling Linkerd connectivity")

	// Remove traffic management configurations
	if err := lp.removeTrafficManagement(ctx, clientset); err != nil {
		return err
	}

	// Remove service mesh configurations
	if err := lp.removeServiceMesh(ctx, clientset); err != nil {
		return err
	}

	lp.logger.Info("Linkerd connectivity uninstallation completed")
	return nil
}

// Status gets the status of Linkerd connectivity
func (lp *LinkerdProvider) Status(ctx context.Context, clientset *kubernetes.Clientset) (ConnectivityStatus, error) {
	status := ConnectivityStatus{
		Type:        ConnectivityTypeLinkerd,
		State:       "unknown",
		Healthy:     false,
		LastChecked: time.Now().Format(time.RFC3339),
	}

	// Check for Linkerd configurations
	configMaps, err := clientset.CoreV1().ConfigMaps("linkerd").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true",
	})
	if err != nil {
		if !isNotFoundError(err) {
			status.ErrorMessage = fmt.Sprintf("failed to check Linkerd configs: %v", err)
			return status, nil
		}
	}

	// Check for Linkerd-related services
	services, err := clientset.CoreV1().Services("linkerd").List(ctx, metav1.ListOptions{
		LabelSelector: "linkerd.io/control-plane-component",
	})
	if err != nil {
		if !isNotFoundError(err) {
			status.ErrorMessage = fmt.Sprintf("failed to check Linkerd services: %v", err)
			return status, nil
		}
	}

	status.ServicesCount = len(services.Items)
	status.PoliciesCount = len(configMaps.Items)

	// Check if Linkerd is installed
	if status.ServicesCount > 0 {
		status.State = "linkerd_installed"
		status.Healthy = true
	} else {
		status.State = "linkerd_not_found"
		status.ErrorMessage = "Linkerd does not appear to be installed in the cluster"
	}

	return status, nil
}

// HealthCheck performs health checks for Linkerd connectivity
func (lp *LinkerdProvider) HealthCheck(ctx context.Context, clientset *kubernetes.Clientset) ([]ConnectivityHealthCheck, error) {
	var checks []ConnectivityHealthCheck
	startTime := time.Now()

	// Linkerd installation check
	linkerdCheck := ConnectivityHealthCheck{
		Name:    "linkerd-installation",
		Type:    "linkerd",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	// Check if linkerd namespace exists
	_, err := clientset.CoreV1().Namespaces().Get(ctx, "linkerd", metav1.GetOptions{})
	if err != nil {
		if isNotFoundError(err) {
			linkerdCheck.Message = "linkerd namespace not found - Linkerd may not be installed"
		} else {
			linkerdCheck.Message = fmt.Sprintf("failed to check linkerd namespace: %v", err)
		}
	} else {
		linkerdCheck.Healthy = true
		linkerdCheck.Message = "linkerd namespace found"
	}
	linkerdCheck.Duration = time.Since(startTime).String()
	checks = append(checks, linkerdCheck)

	// Controller health check
	controllerCheck := ConnectivityHealthCheck{
		Name:    "linkerd-controller",
		Type:    "linkerd",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	if linkerdCheck.Healthy {
		pods, err := clientset.CoreV1().Pods("linkerd").List(ctx, metav1.ListOptions{
			LabelSelector: "linkerd.io/control-plane-component=controller",
		})
		if err != nil {
			controllerCheck.Message = fmt.Sprintf("failed to check controller pods: %v", err)
		} else if len(pods.Items) == 0 {
			controllerCheck.Message = "no controller pods found"
		} else {
			healthyPods := 0
			for _, pod := range pods.Items {
				if pod.Status.Phase == "Running" {
					healthyPods++
				}
			}
			if healthyPods > 0 {
				controllerCheck.Healthy = true
				controllerCheck.Message = fmt.Sprintf("%d/%d controller pods running", healthyPods, len(pods.Items))
			} else {
				controllerCheck.Message = "no controller pods are running"
			}
			controllerCheck.Details = map[string]interface{}{
				"totalPods":   len(pods.Items),
				"runningPods": healthyPods,
			}
		}
	} else {
		controllerCheck.Message = "skipped - Linkerd not installed"
	}
	controllerCheck.Duration = time.Since(startTime).String()
	checks = append(checks, controllerCheck)

	// Service mesh configuration check
	meshConfigCheck := ConnectivityHealthCheck{
		Name:    "service-mesh-config",
		Type:    "linkerd",
		LastRun: startTime.Format(time.RFC3339),
		Healthy: false,
	}

	if linkerdCheck.Healthy {
		configMaps, err := clientset.CoreV1().ConfigMaps("linkerd").List(ctx, metav1.ListOptions{
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
		meshConfigCheck.Message = "skipped - Linkerd not installed"
	}
	meshConfigCheck.Duration = time.Since(startTime).String()
	checks = append(checks, meshConfigCheck)

	return checks, nil
}

// installServiceMesh installs Linkerd service mesh configurations
func (lp *LinkerdProvider) installServiceMesh(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	lp.logger.Info("Installing Linkerd service mesh configurations")

	framework := &Framework{}

	// Create trust domain configuration
	var trustDomain string
	if td, exists := config["trustDomain"]; exists {
		if tdStr, ok := td.(string); ok {
			trustDomain = tdStr
		}
	}

	if trustDomain == "" {
		trustDomain = "cluster.local"
	}

	linkerdConfig := map[string]string{
		"trust-domain":    trustDomain,
		"service-mesh":    "enabled",
		"cross-cluster":   "enabled",
		"identity-issuer": "enabled",
	}

	if err := framework.CreateConfigMap(ctx, clientset, "linkerd-mesh-config", "linkerd", linkerdConfig); err != nil {
		return err
	}

	return nil
}

// installTrafficManagement installs Linkerd traffic management configurations
func (lp *LinkerdProvider) installTrafficManagement(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error {
	lp.logger.Info("Installing Linkerd traffic management configurations")

	framework := &Framework{}

	// Create traffic management configuration
	loadBalancing := "ewma"
	if lb, exists := config["loadBalancing"]; exists {
		if lbStr, ok := lb.(string); ok {
			loadBalancing = lbStr
		}
	}

	trafficConfig := map[string]string{
		"load-balancing":    loadBalancing,
		"traffic-splitting": "enabled",
		"service-profiles":  "enabled",
		"authorization":     "enabled",
	}

	if err := framework.CreateConfigMap(ctx, clientset, "linkerd-traffic-config", "linkerd", trafficConfig); err != nil {
		return err
	}

	return nil
}

// removeServiceMesh removes Linkerd service mesh configurations
func (lp *LinkerdProvider) removeServiceMesh(ctx context.Context, clientset *kubernetes.Clientset) error {
	lp.logger.Info("Removing Linkerd service mesh configurations")

	configMaps, err := clientset.CoreV1().ConfigMaps("linkerd").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,linkerd-mesh-config=true",
	})
	if err != nil {
		if !isNotFoundError(err) {
			return err
		}
		return nil // linkerd namespace doesn't exist, nothing to clean up
	}

	for _, cm := range configMaps.Items {
		if err := clientset.CoreV1().ConfigMaps(cm.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
			lp.logger.Warnf("Failed to delete config map %s/%s: %v", cm.Namespace, cm.Name, err)
		}
	}

	return nil
}

// removeTrafficManagement removes Linkerd traffic management configurations
func (lp *LinkerdProvider) removeTrafficManagement(ctx context.Context, clientset *kubernetes.Clientset) error {
	lp.logger.Info("Removing Linkerd traffic management configurations")

	configMaps, err := clientset.CoreV1().ConfigMaps("linkerd").List(ctx, metav1.ListOptions{
		LabelSelector: "connectivity-framework=true,linkerd-traffic-config=true",
	})
	if err != nil {
		if !isNotFoundError(err) {
			return err
		}
		return nil // linkerd namespace doesn't exist, nothing to clean up
	}

	for _, cm := range configMaps.Items {
		if err := clientset.CoreV1().ConfigMaps(cm.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
			lp.logger.Warnf("Failed to delete config map %s/%s: %v", cm.Namespace, cm.Name, err)
		}
	}

	return nil
}
