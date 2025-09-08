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

func TestNewPropagationProvider(t *testing.T) {
	provider := NewPropagationProvider()

	assert.NotNil(t, provider)
	assert.NotNil(t, provider.logger)
	assert.Equal(t, ServiceDiscoveryTypePropagation, provider.Type())
}

func TestPropagationProvider_Type(t *testing.T) {
	provider := NewPropagationProvider()

	assert.Equal(t, ServiceDiscoveryTypePropagation, provider.Type())
}

func TestPropagationProvider_ValidateConfig_Valid(t *testing.T) {
	provider := NewPropagationProvider()

	validConfig := map[string]interface{}{
		"enabled":      true,
		"clusters":     []interface{}{"cluster1", "cluster2"},
		"namespaces":   []interface{}{"default", "kube-public"},
		"syncInterval": "5m",
	}

	err := provider.ValidateConfig(validConfig)
	assert.NoError(t, err)
}

func TestPropagationProvider_ValidateConfig_Disabled(t *testing.T) {
	provider := NewPropagationProvider()

	disabledConfig := map[string]interface{}{
		"enabled": false,
	}

	err := provider.ValidateConfig(disabledConfig)
	assert.NoError(t, err)
}

func TestPropagationProvider_ValidateConfig_MissingClusters(t *testing.T) {
	provider := NewPropagationProvider()

	invalidConfig := map[string]interface{}{
		"enabled":    true,
		"namespaces": []interface{}{"default"},
	}

	err := provider.ValidateConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service propagation configuration must specify at least one cluster")
}

func TestPropagationProvider_Install(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	config := map[string]interface{}{
		"enabled":    true,
		"clusters":   []interface{}{"cluster1"},
		"namespaces": []interface{}{"default"},
	}

	err := provider.Install(context.Background(), clientset, config)
	assert.NoError(t, err)

	// Verify ConfigMap was created
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "service-propagation-config", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, configMap)
}

func TestPropagationProvider_Uninstall(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	// Create resources to simulate previous installation
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagation-config",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(context.Background(), configMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create service account
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagator",
			Namespace: "kube-system",
		},
	}
	_, err = clientset.CoreV1().ServiceAccounts("kube-system").Create(context.Background(), sa, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.Uninstall(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify ConfigMap was deleted
	_, err = clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "service-propagation-config", metav1.GetOptions{})
	assert.Error(t, err) // Should not exist
}

func TestPropagationProvider_Status(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	// Create a deployment to simulate running service propagator
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagator",
			Namespace: "kube-system",
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      1,
			ReadyReplicas: 1,
		},
	}
	_, err := clientset.AppsV1().Deployments("kube-system").Create(context.Background(), deployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create some propagated services
	service1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "propagated-service-1",
			Namespace: "default",
			Labels: map[string]string{
				"service-discovery/propagated": "true",
			},
		},
	}
	_, err = clientset.CoreV1().Services("default").Create(context.Background(), service1, metav1.CreateOptions{})
	assert.NoError(t, err)

	status, err := provider.Status(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Equal(t, ServiceDiscoveryTypePropagation, status.Type)
	assert.Equal(t, "running", status.State)
	assert.True(t, status.Healthy)
	assert.Equal(t, 1, status.ServicesCount)
}

func TestPropagationProvider_Status_NoDeployment(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	status, err := provider.Status(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Equal(t, ServiceDiscoveryTypePropagation, status.Type)
	assert.Equal(t, "unknown", status.State)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.ErrorMessage, "failed to get service propagator deployment")
}

func TestPropagationProvider_HealthCheck(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	// Create a deployment to simulate running service propagator
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagator",
			Namespace: "kube-system",
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      1,
			ReadyReplicas: 1,
		},
	}
	_, err := clientset.AppsV1().Deployments("kube-system").Create(context.Background(), deployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create state ConfigMap
	stateConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagation-state",
			Namespace: "kube-system",
		},
	}
	_, err = clientset.CoreV1().ConfigMaps("kube-system").Create(context.Background(), stateConfigMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	checks, err := provider.HealthCheck(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Greater(t, len(checks), 0)

	// Check that we have the expected health checks
	checkNames := make(map[string]bool)
	for _, check := range checks {
		checkNames[check.Name] = true
	}

	assert.True(t, checkNames["service-propagator-deployment"], "Should have service-propagator-deployment check")
	assert.True(t, checkNames["propagation-state"], "Should have propagation-state check")
	assert.True(t, checkNames["service-propagation-sync"], "Should have service-propagation-sync check")
}

func TestPropagationProvider_parsePropagationConfig(t *testing.T) {
	provider := NewPropagationProvider()

	config := map[string]interface{}{
		"enabled":      true,
		"clusters":     []interface{}{"cluster1", "cluster2"},
		"namespaces":   []interface{}{"default", "kube-public"},
		"syncInterval": "10m",
		"selectorLabels": map[string]interface{}{
			"app": "test",
		},
		"excludeLabels": map[string]interface{}{
			"skip": "true",
		},
	}

	propagationConfig, err := provider.parsePropagationConfig(config)
	assert.NoError(t, err)
	assert.True(t, propagationConfig.Enabled)
	assert.Equal(t, []string{"cluster1", "cluster2"}, propagationConfig.Clusters)
	assert.Equal(t, []string{"default", "kube-public"}, propagationConfig.Namespaces)
	assert.Equal(t, map[string]string{"app": "test"}, propagationConfig.SelectorLabels)
	assert.Equal(t, map[string]string{"skip": "true"}, propagationConfig.ExcludeLabels)
}

func TestPropagationProvider_createServicePropagatorDeployment(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	propagationConfig := &ServicePropagationConfig{
		Enabled:    true,
		Clusters:   []string{"cluster1"},
		Namespaces: []string{"default"},
	}

	err := provider.createServicePropagatorDeployment(context.Background(), clientset, propagationConfig)
	assert.NoError(t, err)

	// Verify deployment was created
	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "service-propagator", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, "service-propagator", deployment.Name)
	assert.Equal(t, "kube-system", deployment.Namespace)
}

func TestPropagationProvider_deleteServicePropagatorDeployment(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	// Create a deployment first
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagator",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.AppsV1().Deployments("kube-system").Create(context.Background(), deployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.deleteServicePropagatorDeployment(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify deployment was deleted
	_, err = clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "service-propagator", metav1.GetOptions{})
	assert.Error(t, err) // Should not exist
}

func TestPropagationProvider_createServicePropagatorRBAC(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	err := provider.createServicePropagatorRBAC(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify service account was created
	sa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "service-propagator", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, sa)

	// Verify cluster role was created
	cr, err := clientset.RbacV1().ClusterRoles().Get(context.Background(), "service-propagator", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, cr)

	// Verify cluster role binding was created
	crb, err := clientset.RbacV1().ClusterRoleBindings().Get(context.Background(), "service-propagator", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, crb)
}

func TestPropagationProvider_deleteServicePropagatorRBAC(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	// Create RBAC resources first
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagator",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.CoreV1().ServiceAccounts("kube-system").Create(context.Background(), sa, metav1.CreateOptions{})
	assert.NoError(t, err)

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "service-propagator",
		},
	}
	_, err = clientset.RbacV1().ClusterRoles().Create(context.Background(), cr, metav1.CreateOptions{})
	assert.NoError(t, err)

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "service-propagator",
		},
	}
	_, err = clientset.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.deleteServicePropagatorRBAC(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify resources were deleted
	_, err = clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "service-propagator", metav1.GetOptions{})
	assert.Error(t, err)

	_, err = clientset.RbacV1().ClusterRoles().Get(context.Background(), "service-propagator", metav1.GetOptions{})
	assert.Error(t, err)

	_, err = clientset.RbacV1().ClusterRoleBindings().Get(context.Background(), "service-propagator", metav1.GetOptions{})
	assert.Error(t, err)
}

func TestPropagationProvider_initializePropagationState(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	err := provider.initializePropagationState(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify ConfigMap was created
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "service-propagation-state", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, configMap)
	assert.Contains(t, configMap.Data, "last-sync")
	assert.Contains(t, configMap.Data, "services")
	assert.Contains(t, configMap.Data, "status")
}

func TestPropagationProvider_checkPropagationState(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	// Create state ConfigMap
	stateConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagation-state",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(context.Background(), stateConfigMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.checkPropagationState(context.Background(), clientset)
	assert.NoError(t, err)
}

func TestPropagationProvider_checkPropagationState_MissingConfigMap(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	err := provider.checkPropagationState(context.Background(), clientset)
	assert.Error(t, err)
}

func TestPropagationProvider_checkServicePropagationSync(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	// Create a propagated service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "propagated-service",
			Namespace: "default",
			Labels: map[string]string{
				"service-discovery/propagated": "true",
			},
		},
	}
	_, err := clientset.CoreV1().Services("default").Create(context.Background(), service, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.checkServicePropagationSync(context.Background(), clientset)
	assert.NoError(t, err)
}

func TestPropagationProvider_checkServicePropagationSync_NoServices(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	err := provider.checkServicePropagationSync(context.Background(), clientset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no propagated services found")
}

func TestPropagationProvider_cleanupPropagatedServices(t *testing.T) {
	provider := NewPropagationProvider()
	clientset := fake.NewSimpleClientset()

	// Create propagated services and endpoints
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "propagated-service",
			Namespace: "default",
			Labels: map[string]string{
				"service-discovery/propagated": "true",
			},
		},
	}
	_, err := clientset.CoreV1().Services("default").Create(context.Background(), service, metav1.CreateOptions{})
	assert.NoError(t, err)

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "propagated-service",
			Namespace: "default",
			Labels: map[string]string{
				"service-discovery/propagated": "true",
			},
		},
	}
	_, err = clientset.CoreV1().Endpoints("default").Create(context.Background(), endpoints, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.cleanupPropagatedServices(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify services were deleted
	_, err = clientset.CoreV1().Services("default").Get(context.Background(), "propagated-service", metav1.GetOptions{})
	assert.Error(t, err)

	// Verify endpoints were deleted
	_, err = clientset.CoreV1().Endpoints("default").Get(context.Background(), "propagated-service", metav1.GetOptions{})
	assert.Error(t, err)
}
