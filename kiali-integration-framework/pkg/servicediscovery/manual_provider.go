package servicediscovery

import (
	"context"
	"fmt"
	"time"

	"github.com/kiali/kiali-integration-framework/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ManualProvider implements manual service discovery configuration
type ManualProvider struct {
	logger *utils.Logger
}

// NewManualProvider creates a new manual provider
func NewManualProvider() *ManualProvider {
	return &ManualProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Type returns the service discovery type
func (m *ManualProvider) Type() ServiceDiscoveryType {
	return ServiceDiscoveryTypeManual
}

// ValidateConfig validates the manual service discovery configuration
func (m *ManualProvider) ValidateConfig(config map[string]interface{}) error {
	// Manual provider accepts any configuration as it's custom
	// Basic validation to ensure the config is not nil
	if config == nil {
		return fmt.Errorf("manual service discovery configuration cannot be nil")
	}

	return nil
}

// Install installs manual service discovery configuration
func (m *ManualProvider) Install(ctx context.Context, clientset kubernetes.Interface, config map[string]interface{}) error {
	m.logger.Info("Installing manual service discovery configuration")

	// Create a ConfigMap to store the manual configuration
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "manual-service-discovery-config",
			Namespace: "kube-system",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
				"service-discovery":            "true",
				"discovery-type":               "manual",
			},
		},
		Data: map[string]string{
			"manual-config.yaml": fmt.Sprintf("%v", config),
			"installed-at":       time.Now().Format(time.RFC3339),
		},
	}

	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return utils.WrapError(err, utils.ErrCodeComponentInstallFailed,
			"failed to create manual service discovery config map")
	}

	m.logger.Infof("Created manual service discovery configuration ConfigMap")
	m.logger.Info("Successfully installed manual service discovery configuration")
	return nil
}

// Uninstall uninstalls manual service discovery configuration
func (m *ManualProvider) Uninstall(ctx context.Context, clientset kubernetes.Interface) error {
	m.logger.Info("Uninstalling manual service discovery configuration")

	// Delete the manual configuration ConfigMap
	if err := clientset.CoreV1().ConfigMaps("kube-system").Delete(ctx, "manual-service-discovery-config", metav1.DeleteOptions{}); err != nil {
		m.logger.Warnf("Failed to delete manual service discovery config map: %v", err)
	}

	m.logger.Info("Successfully uninstalled manual service discovery configuration")
	return nil
}

// Status returns the status of manual service discovery configuration
func (m *ManualProvider) Status(ctx context.Context, clientset kubernetes.Interface) (ServiceDiscoveryStatus, error) {
	status := ServiceDiscoveryStatus{
		Type:        ServiceDiscoveryTypeManual,
		State:       "configured",
		Healthy:     true,
		LastChecked: time.Now(),
	}

	// Check if manual configuration ConfigMap exists
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "manual-service-discovery-config", metav1.GetOptions{})
	if err != nil {
		status.State = "not_configured"
		status.Healthy = false
		status.ErrorMessage = fmt.Sprintf("manual configuration not found: %v", err)
	}

	return status, nil
}

// HealthCheck performs health checks for manual service discovery
func (m *ManualProvider) HealthCheck(ctx context.Context, clientset kubernetes.Interface) ([]ServiceDiscoveryHealthCheck, error) {
	var checks []ServiceDiscoveryHealthCheck

	// Manual configuration check
	configCheck := ServiceDiscoveryHealthCheck{
		Name:    "manual-configuration",
		Type:    "configuration",
		LastRun: time.Now(),
	}

	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "manual-service-discovery-config", metav1.GetOptions{})
	if err != nil {
		configCheck.Healthy = false
		configCheck.Message = fmt.Sprintf("Manual configuration ConfigMap not found: %v", err)
	} else {
		configCheck.Healthy = true
		configCheck.Message = "Manual configuration ConfigMap exists"

		// Add configuration details
		configCheck.Details = map[string]interface{}{
			"config-size":  len(configMap.Data),
			"installed-at": configMap.Data["installed-at"],
		}
	}
	configCheck.Duration = time.Since(configCheck.LastRun)

	checks = append(checks, configCheck)

	return checks, nil
}
