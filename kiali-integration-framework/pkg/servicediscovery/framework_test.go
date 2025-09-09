package servicediscovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// MockServiceDiscoveryProvider is a mock implementation of ServiceDiscoveryProvider
type MockServiceDiscoveryProvider struct {
	mock.Mock
}

func (m *MockServiceDiscoveryProvider) Type() ServiceDiscoveryType {
	args := m.Called()
	return args.Get(0).(ServiceDiscoveryType)
}

func (m *MockServiceDiscoveryProvider) Install(ctx context.Context, clientset kubernetes.Interface, config map[string]interface{}) error {
	args := m.Called(ctx, clientset, config)
	return args.Error(0)
}

func (m *MockServiceDiscoveryProvider) Uninstall(ctx context.Context, clientset kubernetes.Interface) error {
	args := m.Called(ctx, clientset)
	return args.Error(0)
}

func (m *MockServiceDiscoveryProvider) Status(ctx context.Context, clientset kubernetes.Interface) (ServiceDiscoveryStatus, error) {
	args := m.Called(ctx, clientset)
	return args.Get(0).(ServiceDiscoveryStatus), args.Error(1)
}

func (m *MockServiceDiscoveryProvider) HealthCheck(ctx context.Context, clientset kubernetes.Interface) ([]ServiceDiscoveryHealthCheck, error) {
	args := m.Called(ctx, clientset)
	return args.Get(0).([]ServiceDiscoveryHealthCheck), args.Error(1)
}

func (m *MockServiceDiscoveryProvider) ValidateConfig(config map[string]interface{}) error {
	args := m.Called(config)
	return args.Error(0)
}

func TestNewFramework(t *testing.T) {
	framework := NewFramework()

	assert.NotNil(t, framework)
	assert.NotNil(t, framework.logger)
}

func TestFramework_RegisterProvider(t *testing.T) {
	framework := NewFramework()
	provider := &MockServiceDiscoveryProvider{}

	provider.On("Type").Return(ServiceDiscoveryTypeDNS)

	framework.RegisterProvider(provider)

	// Verify provider was registered
	retrieved, err := framework.GetProvider(ServiceDiscoveryTypeDNS)
	assert.NoError(t, err)
	assert.Equal(t, provider, retrieved)
}

func TestFramework_GetProvider_NotFound(t *testing.T) {
	framework := NewFramework()

	_, err := framework.GetProvider(ServiceDiscoveryTypeDNS)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service discovery provider not found")
}

func TestFramework_InstallServiceDiscovery(t *testing.T) {
	framework := NewFramework()
	provider := &MockServiceDiscoveryProvider{}
	clientset := fake.NewSimpleClientset()

	provider.On("Type").Return(ServiceDiscoveryTypeDNS)
	provider.On("ValidateConfig", mock.Anything).Return(nil)
	provider.On("Install", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	framework.RegisterProvider(provider)

	err := framework.InstallServiceDiscovery(context.Background(), clientset, ServiceDiscoveryTypeDNS, map[string]interface{}{})

	assert.NoError(t, err)
	provider.AssertExpectations(t)
}

func TestFramework_UninstallServiceDiscovery(t *testing.T) {
	framework := NewFramework()
	provider := &MockServiceDiscoveryProvider{}
	clientset := fake.NewSimpleClientset()

	provider.On("Type").Return(ServiceDiscoveryTypeDNS)
	provider.On("Uninstall", mock.Anything, mock.Anything).Return(nil)

	framework.RegisterProvider(provider)

	err := framework.UninstallServiceDiscovery(context.Background(), clientset, ServiceDiscoveryTypeDNS)

	assert.NoError(t, err)
	provider.AssertExpectations(t)
}

func TestFramework_GetServiceDiscoveryStatus(t *testing.T) {
	framework := NewFramework()
	provider := &MockServiceDiscoveryProvider{}
	clientset := fake.NewSimpleClientset()
	expectedStatus := ServiceDiscoveryStatus{
		Type:    ServiceDiscoveryTypeDNS,
		State:   "running",
		Healthy: true,
	}

	provider.On("Type").Return(ServiceDiscoveryTypeDNS)
	provider.On("Status", mock.Anything, mock.Anything).Return(expectedStatus, nil)

	framework.RegisterProvider(provider)

	status, err := framework.GetServiceDiscoveryStatus(context.Background(), clientset, ServiceDiscoveryTypeDNS)

	assert.NoError(t, err)
	assert.Equal(t, expectedStatus, status)
	provider.AssertExpectations(t)
}

func TestFramework_RunHealthChecks(t *testing.T) {
	framework := NewFramework()
	provider := &MockServiceDiscoveryProvider{}
	clientset := fake.NewSimpleClientset()
	expectedChecks := []ServiceDiscoveryHealthCheck{
		{
			Name:    "test-check",
			Type:    "test",
			Healthy: true,
			Message: "Test passed",
		},
	}

	provider.On("Type").Return(ServiceDiscoveryTypeDNS)
	provider.On("HealthCheck", mock.Anything, mock.Anything).Return(expectedChecks, nil)

	framework.RegisterProvider(provider)

	checks, err := framework.RunHealthChecks(context.Background(), clientset, ServiceDiscoveryTypeDNS)

	assert.NoError(t, err)
	assert.Equal(t, expectedChecks, checks)
	provider.AssertExpectations(t)
}

func TestFramework_ValidateServiceDiscoveryConfig(t *testing.T) {
	framework := NewFramework()

	// Register a DNS provider for validation
	dnsProvider := NewDNSProvider()
	framework.RegisterProvider(dnsProvider)

	// Test valid config with complete DNS configuration
	validConfig := &ServiceDiscoveryConfig{
		Enabled:  true,
		Type:     ServiceDiscoveryTypeDNS,
		Clusters: []string{"cluster1", "cluster2"},
		Config: map[string]interface{}{
			"enabled":     true,
			"clusters":    []interface{}{"cluster1", "cluster2"},
			"nameservers": []interface{}{"10.96.0.10", "8.8.8.8"},
		},
	}

	err := framework.ValidateServiceDiscoveryConfig(validConfig)
	assert.NoError(t, err)

	// Test disabled config
	disabledConfig := &ServiceDiscoveryConfig{
		Enabled: false,
	}

	err = framework.ValidateServiceDiscoveryConfig(disabledConfig)
	assert.NoError(t, err)

	// Test invalid config - missing type
	invalidConfig := &ServiceDiscoveryConfig{
		Enabled:  true,
		Clusters: []string{"cluster1"},
	}

	err = framework.ValidateServiceDiscoveryConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service discovery type is required")
}

func TestFramework_GetServiceDiscoveryInfo(t *testing.T) {
	framework := NewFramework()
	clientset := fake.NewSimpleClientset()

	// Create test services
	services := []*corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service-1",
				Namespace: "default",
				Labels: map[string]string{
					"app": "test",
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "http",
						Port: 80,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service-2",
				Namespace: "kube-system",
				Labels: map[string]string{
					"service-discovery/skip": "true",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubernetes",
				Namespace: "default",
			},
		},
	}

	for _, svc := range services {
		_, err := clientset.CoreV1().Services(svc.Namespace).Create(context.Background(), svc, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	endpoints, err := framework.GetServiceDiscoveryInfo(context.Background(), clientset, "default")

	assert.NoError(t, err)
	assert.Len(t, endpoints, 1) // Only test-service-1 should be included
	assert.Equal(t, "test-service-1", endpoints[0].Name)
	assert.Equal(t, "default", endpoints[0].Namespace)
}

func TestFramework_GenerateResolvConf(t *testing.T) {
	framework := NewFramework()

	dnsConfig := &DNSConfig{
		Enabled:       true,
		Clusters:      []string{"cluster1", "cluster2"},
		SearchDomains: []string{"cluster.local", "svc.cluster.local"},
		Nameservers:   []string{"10.96.0.10", "8.8.8.8"},
		TTL:           30,
	}

	resolvConf := framework.generateResolvConf(dnsConfig)

	assert.Contains(t, resolvConf, "nameserver 10.96.0.10")
	assert.Contains(t, resolvConf, "nameserver 8.8.8.8")
	assert.Contains(t, resolvConf, "search cluster.local svc.cluster.local")
	assert.Contains(t, resolvConf, "options timeout:30")
}

func TestFramework_CreateDNSConfigMap(t *testing.T) {
	framework := NewFramework()
	clientset := fake.NewSimpleClientset()

	dnsConfig := &DNSConfig{
		Enabled:       true,
		Clusters:      []string{"cluster1"},
		SearchDomains: []string{"cluster.local"},
		Nameservers:   []string{"10.96.0.10"},
	}

	err := framework.CreateDNSConfigMap(context.Background(), clientset, "test-dns-config", "kube-system", dnsConfig)

	assert.NoError(t, err)

	// Verify ConfigMap was created
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "test-dns-config", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, configMap)
	assert.Contains(t, configMap.Data, "resolv.conf")
	assert.Contains(t, configMap.Data, "dns-config")
}

func TestFramework_CreateAPIServerConfigMap(t *testing.T) {
	framework := NewFramework()
	clientset := fake.NewSimpleClientset()

	apiConfig := &APIServerConfig{
		Enabled:      true,
		Clusters:     []string{"cluster1"},
		APIServerURL: "https://api.cluster1.local:6443",
	}

	err := framework.CreateAPIServerConfigMap(context.Background(), clientset, "test-api-config", "kube-system", apiConfig)

	assert.NoError(t, err)

	// Verify ConfigMap was created
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "test-api-config", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, configMap)
	assert.Contains(t, configMap.Data, "api-server-config")
}

func TestFramework_CreateServicePropagationConfigMap(t *testing.T) {
	framework := NewFramework()
	clientset := fake.NewSimpleClientset()

	propagationConfig := &ServicePropagationConfig{
		Enabled:      true,
		Clusters:     []string{"cluster1", "cluster2"},
		Namespaces:   []string{"default", "kube-public"},
		SyncInterval: 300000000000, // 5 minutes in nanoseconds
	}

	err := framework.CreateServicePropagationConfigMap(context.Background(), clientset, "test-propagation-config", "kube-system", propagationConfig)

	assert.NoError(t, err)

	// Verify ConfigMap was created
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "test-propagation-config", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, configMap)
	assert.Contains(t, configMap.Data, "propagation-config")
}
