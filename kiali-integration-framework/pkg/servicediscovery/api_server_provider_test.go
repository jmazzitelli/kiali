package servicediscovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewAPIServerProvider(t *testing.T) {
	provider := NewAPIServerProvider()

	assert.NotNil(t, provider)
	assert.NotNil(t, provider.logger)
	assert.Equal(t, ServiceDiscoveryTypeAPIServer, provider.Type())
}

func TestAPIServerProvider_Type(t *testing.T) {
	provider := NewAPIServerProvider()

	assert.Equal(t, ServiceDiscoveryTypeAPIServer, provider.Type())
}

func TestAPIServerProvider_ValidateConfig_Valid(t *testing.T) {
	provider := NewAPIServerProvider()

	validConfig := map[string]interface{}{
		"enabled":      true,
		"clusters":     []interface{}{"cluster1", "cluster2"},
		"apiServerUrl": "https://api.cluster1.local:6443",
		"caCert":       "LS0tLS1CRUdJTi...",
		"clientCert":   "LS0tLS1CRUdJTi...",
		"clientKey":    "LS0tLS1CRUdJTi...",
	}

	err := provider.ValidateConfig(validConfig)
	assert.NoError(t, err)
}

func TestAPIServerProvider_ValidateConfig_Disabled(t *testing.T) {
	provider := NewAPIServerProvider()

	disabledConfig := map[string]interface{}{
		"enabled": false,
	}

	err := provider.ValidateConfig(disabledConfig)
	assert.NoError(t, err)
}

func TestAPIServerProvider_ValidateConfig_MissingClusters(t *testing.T) {
	provider := NewAPIServerProvider()

	invalidConfig := map[string]interface{}{
		"enabled":      true,
		"apiServerUrl": "https://api.cluster1.local:6443",
	}

	err := provider.ValidateConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API server configuration must specify at least one cluster")
}

func TestAPIServerProvider_ValidateConfig_MissingURL(t *testing.T) {
	provider := NewAPIServerProvider()

	invalidConfig := map[string]interface{}{
		"enabled":  true,
		"clusters": []interface{}{"cluster1"},
	}

	err := provider.ValidateConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API server URL is required")
}

func TestAPIServerProvider_ValidateConfig_InvalidURL(t *testing.T) {
	provider := NewAPIServerProvider()

	invalidConfig := map[string]interface{}{
		"enabled":      true,
		"clusters":     []interface{}{"cluster1"},
		"apiServerUrl": "invalid-url",
	}

	err := provider.ValidateConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid API server URL")
}

func TestAPIServerProvider_isValidURL(t *testing.T) {
	provider := NewAPIServerProvider()

	tests := []struct {
		url      string
		expected bool
	}{
		{"https://api.example.com:6443", true},
		{"http://api.example.com", true},
		{"https://api.example.com", true},
		{"", false},
		{"not-a-url", false},
		{"ftp://api.example.com", false},
		{"api.example.com", false},
	}

	for _, test := range tests {
		result := provider.isValidURL(test.url)
		assert.Equal(t, test.expected, result, "Failed for URL: %s", test.url)
	}
}

func TestAPIServerProvider_Install(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	config := map[string]interface{}{
		"enabled":      true,
		"clusters":     []interface{}{"cluster1"},
		"apiServerUrl": "https://api.cluster1.local:6443",
	}

	err := provider.Install(context.Background(), clientset, config)
	assert.NoError(t, err)

	// Verify ConfigMap was created
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "api-server-aggregation-config", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, configMap)
}

func TestAPIServerProvider_Uninstall(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	// Create resources to simulate previous installation
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server-aggregation-config",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(context.Background(), configMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.Uninstall(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify ConfigMap was deleted
	_, err = clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "api-server-aggregation-config", metav1.GetOptions{})
	assert.Error(t, err) // Should not exist
}

func TestAPIServerProvider_Status(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	// Create a deployment to simulate running API server aggregator
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server-aggregator",
			Namespace: "kube-system",
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      1,
			ReadyReplicas: 1,
		},
	}
	_, err := clientset.AppsV1().Deployments("kube-system").Create(context.Background(), deployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	status, err := provider.Status(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Equal(t, ServiceDiscoveryTypeAPIServer, status.Type)
	assert.Equal(t, "running", status.State)
	assert.True(t, status.Healthy)
}

func TestAPIServerProvider_Status_NoDeployment(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	status, err := provider.Status(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Equal(t, ServiceDiscoveryTypeAPIServer, status.Type)
	assert.Equal(t, "unknown", status.State)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.ErrorMessage, "failed to get API server aggregator deployment")
}

func TestAPIServerProvider_HealthCheck(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	// Create a deployment to simulate running API server aggregator
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server-aggregator",
			Namespace: "kube-system",
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      1,
			ReadyReplicas: 1,
		},
	}
	_, err := clientset.AppsV1().Deployments("kube-system").Create(context.Background(), deployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	checks, err := provider.HealthCheck(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Greater(t, len(checks), 0)

	// Check that we have the expected health checks
	checkNames := make(map[string]bool)
	for _, check := range checks {
		checkNames[check.Name] = true
	}

	assert.True(t, checkNames["api-server-aggregator-deployment"], "Should have api-server-aggregator-deployment check")
	assert.True(t, checkNames["api-server-connectivity"], "Should have api-server-connectivity check")
	assert.True(t, checkNames["api-server-rbac"], "Should have api-server-rbac check")
}

func TestAPIServerProvider_parseAPIServerConfig(t *testing.T) {
	provider := NewAPIServerProvider()

	config := map[string]interface{}{
		"enabled":      true,
		"clusters":     []interface{}{"cluster1", "cluster2"},
		"apiServerUrl": "https://api.cluster1.local:6443",
		"caCert":       "ca-cert-data",
		"clientCert":   "client-cert-data",
		"clientKey":    "client-key-data",
	}

	apiConfig, err := provider.parseAPIServerConfig(config)
	assert.NoError(t, err)
	assert.True(t, apiConfig.Enabled)
	assert.Equal(t, []string{"cluster1", "cluster2"}, apiConfig.Clusters)
	assert.Equal(t, "https://api.cluster1.local:6443", apiConfig.APIServerURL)
	assert.Equal(t, "ca-cert-data", apiConfig.CACert)
	assert.Equal(t, "client-cert-data", apiConfig.ClientCert)
	assert.Equal(t, "client-key-data", apiConfig.ClientKey)
}

func TestAPIServerProvider_createAPIServerAggregatorDeployment(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	apiConfig := &APIServerConfig{
		Enabled:      true,
		Clusters:     []string{"cluster1"},
		APIServerURL: "https://api.cluster1.local:6443",
	}

	err := provider.createAPIServerAggregatorDeployment(context.Background(), clientset, apiConfig)
	assert.NoError(t, err)

	// Verify deployment was created
	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "api-server-aggregator", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, "api-server-aggregator", deployment.Name)
	assert.Equal(t, "kube-system", deployment.Namespace)
}

func TestAPIServerProvider_deleteAPIServerAggregatorDeployment(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	// Create a deployment first
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server-aggregator",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.AppsV1().Deployments("kube-system").Create(context.Background(), deployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.deleteAPIServerAggregatorDeployment(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify deployment was deleted
	_, err = clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "api-server-aggregator", metav1.GetOptions{})
	assert.Error(t, err) // Should not exist
}

func TestAPIServerProvider_createAPIServerRBAC(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	err := provider.createAPIServerRBAC(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify service account was created
	sa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "api-server-aggregator", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, sa)

	// Verify cluster role was created
	cr, err := clientset.RbacV1().ClusterRoles().Get(context.Background(), "api-server-aggregator", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, cr)

	// Verify cluster role binding was created
	crb, err := clientset.RbacV1().ClusterRoleBindings().Get(context.Background(), "api-server-aggregator", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, crb)
}

func TestAPIServerProvider_deleteAPIServerRBAC(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	// Create RBAC resources first
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server-aggregator",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.CoreV1().ServiceAccounts("kube-system").Create(context.Background(), sa, metav1.CreateOptions{})
	assert.NoError(t, err)

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-server-aggregator",
		},
	}
	_, err = clientset.RbacV1().ClusterRoles().Create(context.Background(), cr, metav1.CreateOptions{})
	assert.NoError(t, err)

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-server-aggregator",
		},
	}
	_, err = clientset.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.deleteAPIServerRBAC(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify resources were deleted
	_, err = clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "api-server-aggregator", metav1.GetOptions{})
	assert.Error(t, err)

	_, err = clientset.RbacV1().ClusterRoles().Get(context.Background(), "api-server-aggregator", metav1.GetOptions{})
	assert.Error(t, err)

	_, err = clientset.RbacV1().ClusterRoleBindings().Get(context.Background(), "api-server-aggregator", metav1.GetOptions{})
	assert.Error(t, err)
}

func TestAPIServerProvider_configureAPIServerAggregation(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	apiConfig := &APIServerConfig{
		Enabled:      true,
		Clusters:     []string{"cluster1"},
		APIServerURL: "https://api.cluster1.local:6443",
	}

	err := provider.configureAPIServerAggregation(context.Background(), clientset, apiConfig)
	assert.NoError(t, err)

	// Verify ConfigMap was created
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "api-server-aggregation-setup", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, configMap)
	assert.Contains(t, configMap.Data, "aggregation-config")
}

func TestAPIServerProvider_checkAPIServerRBAC(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	// Create RBAC resources
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server-aggregator",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.CoreV1().ServiceAccounts("kube-system").Create(context.Background(), sa, metav1.CreateOptions{})
	assert.NoError(t, err)

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-server-aggregator",
		},
	}
	_, err = clientset.RbacV1().ClusterRoles().Create(context.Background(), cr, metav1.CreateOptions{})
	assert.NoError(t, err)

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-server-aggregator",
		},
	}
	_, err = clientset.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.checkAPIServerRBAC(context.Background(), clientset)
	assert.NoError(t, err)
}

func TestAPIServerProvider_checkAPIServerRBAC_MissingResources(t *testing.T) {
	provider := NewAPIServerProvider()
	clientset := fake.NewSimpleClientset()

	err := provider.checkAPIServerRBAC(context.Background(), clientset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service account not found")
}
