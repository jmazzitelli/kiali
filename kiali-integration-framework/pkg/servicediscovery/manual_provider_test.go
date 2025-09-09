package servicediscovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewManualProvider(t *testing.T) {
	provider := NewManualProvider()

	assert.NotNil(t, provider)
	assert.NotNil(t, provider.logger)
	assert.Equal(t, ServiceDiscoveryTypeManual, provider.Type())
}

func TestManualProvider_Type(t *testing.T) {
	provider := NewManualProvider()

	assert.Equal(t, ServiceDiscoveryTypeManual, provider.Type())
}

func TestManualProvider_ValidateConfig_Valid(t *testing.T) {
	provider := NewManualProvider()

	validConfig := map[string]interface{}{
		"enabled":   true,
		"customKey": "customValue",
		"nested": map[string]interface{}{
			"key": "value",
		},
	}

	err := provider.ValidateConfig(validConfig)
	assert.NoError(t, err)
}

func TestManualProvider_ValidateConfig_Empty(t *testing.T) {
	provider := NewManualProvider()

	emptyConfig := map[string]interface{}{}

	err := provider.ValidateConfig(emptyConfig)
	assert.NoError(t, err)
}

func TestManualProvider_ValidateConfig_Nil(t *testing.T) {
	provider := NewManualProvider()

	err := provider.ValidateConfig(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestManualProvider_Install(t *testing.T) {
	provider := NewManualProvider()
	clientset := fake.NewSimpleClientset()

	config := map[string]interface{}{
		"enabled":     true,
		"customKey":   "customValue",
		"description": "Test manual configuration",
	}

	err := provider.Install(context.Background(), clientset, config)
	assert.NoError(t, err)

	// Verify ConfigMap was created
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "manual-service-discovery-config", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, configMap)
	assert.Contains(t, configMap.Data, "manual-config.yaml")
	assert.Contains(t, configMap.Data, "installed-at")
}

func TestManualProvider_Uninstall(t *testing.T) {
	provider := NewManualProvider()
	clientset := fake.NewSimpleClientset()

	// Create a ConfigMap to simulate previous installation
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "manual-service-discovery-config",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(context.Background(), configMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.Uninstall(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify ConfigMap was deleted
	_, err = clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "manual-service-discovery-config", metav1.GetOptions{})
	assert.Error(t, err) // Should not exist
}

func TestManualProvider_Status(t *testing.T) {
	provider := NewManualProvider()
	clientset := fake.NewSimpleClientset()

	// Create a ConfigMap to simulate configured state
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "manual-service-discovery-config",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"installed-at": "2024-01-01T00:00:00Z",
		},
	}
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(context.Background(), configMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	status, err := provider.Status(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Equal(t, ServiceDiscoveryTypeManual, status.Type)
	assert.Equal(t, "configured", status.State)
	assert.True(t, status.Healthy)
}

func TestManualProvider_Status_NotConfigured(t *testing.T) {
	provider := NewManualProvider()
	clientset := fake.NewSimpleClientset()

	status, err := provider.Status(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Equal(t, ServiceDiscoveryTypeManual, status.Type)
	assert.Equal(t, "not_configured", status.State)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.ErrorMessage, "manual configuration not found")
}

func TestManualProvider_HealthCheck(t *testing.T) {
	provider := NewManualProvider()
	clientset := fake.NewSimpleClientset()

	// Create a ConfigMap to simulate configured state
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "manual-service-discovery-config",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"manual-config.yaml": "enabled: true\ncustomKey: customValue",
			"installed-at":       "2024-01-01T00:00:00Z",
		},
	}
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(context.Background(), configMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	checks, err := provider.HealthCheck(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Len(t, checks, 1)

	check := checks[0]
	assert.Equal(t, "manual-configuration", check.Name)
	assert.Equal(t, "configuration", check.Type)
	assert.True(t, check.Healthy)
	assert.Contains(t, check.Message, "Manual configuration ConfigMap exists")
	assert.NotNil(t, check.Details)
	assert.Contains(t, check.Details, "config-size")
	assert.Contains(t, check.Details, "installed-at")
}

func TestManualProvider_HealthCheck_NoConfigMap(t *testing.T) {
	provider := NewManualProvider()
	clientset := fake.NewSimpleClientset()

	checks, err := provider.HealthCheck(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Len(t, checks, 1)

	check := checks[0]
	assert.Equal(t, "manual-configuration", check.Name)
	assert.False(t, check.Healthy)
	assert.Contains(t, check.Message, "not found")
}
