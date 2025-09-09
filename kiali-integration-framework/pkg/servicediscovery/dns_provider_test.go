package servicediscovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewDNSProvider(t *testing.T) {
	provider := NewDNSProvider()

	assert.NotNil(t, provider)
	assert.NotNil(t, provider.logger)
	assert.Equal(t, ServiceDiscoveryTypeDNS, provider.Type())
}

func TestDNSProvider_Type(t *testing.T) {
	provider := NewDNSProvider()

	assert.Equal(t, ServiceDiscoveryTypeDNS, provider.Type())
}

func TestDNSProvider_ValidateConfig_Valid(t *testing.T) {
	provider := NewDNSProvider()

	validConfig := map[string]interface{}{
		"enabled":       true,
		"clusters":      []interface{}{"cluster1", "cluster2"},
		"nameservers":   []interface{}{"10.96.0.10", "8.8.8.8"},
		"searchDomains": []interface{}{"cluster.local"},
		"ttl":           30,
	}

	err := provider.ValidateConfig(validConfig)
	assert.NoError(t, err)
}

func TestDNSProvider_ValidateConfig_Disabled(t *testing.T) {
	provider := NewDNSProvider()

	disabledConfig := map[string]interface{}{
		"enabled": false,
	}

	err := provider.ValidateConfig(disabledConfig)
	assert.NoError(t, err)
}

func TestDNSProvider_ValidateConfig_MissingClusters(t *testing.T) {
	provider := NewDNSProvider()

	invalidConfig := map[string]interface{}{
		"enabled":     true,
		"nameservers": []interface{}{"10.96.0.10"},
	}

	err := provider.ValidateConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DNS configuration must specify at least one cluster")
}

func TestDNSProvider_ValidateConfig_MissingNameservers(t *testing.T) {
	provider := NewDNSProvider()

	invalidConfig := map[string]interface{}{
		"enabled":  true,
		"clusters": []interface{}{"cluster1"},
	}

	err := provider.ValidateConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DNS configuration must specify at least one nameserver")
}

func TestDNSProvider_ValidateConfig_InvalidNameserver(t *testing.T) {
	provider := NewDNSProvider()

	invalidConfig := map[string]interface{}{
		"enabled":     true,
		"clusters":    []interface{}{"cluster1"},
		"nameservers": []interface{}{"-invalid-start"},
	}

	err := provider.ValidateConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid nameserver")
}

func TestDNSProvider_isValidHostname(t *testing.T) {
	provider := NewDNSProvider()

	tests := []struct {
		hostname string
		expected bool
	}{
		{"valid-hostname", true},
		{"valid.hostname.com", true},
		{"a.b.c.d.e.f", true},
		{"", false},
		{"a", true},
		{string(make([]byte, 254)), false}, // too long
		{"-invalid-start", false},
		{"invalid-end-", false},
		{"invalid..double.dots", false},
		{"valid-with-numbers123", true},
	}

	for _, test := range tests {
		result := provider.isValidHostname(test.hostname)
		assert.Equal(t, test.expected, result, "Failed for hostname: %s", test.hostname)
	}
}

func TestDNSProvider_Install(t *testing.T) {
	provider := NewDNSProvider()
	clientset := fake.NewSimpleClientset()

	config := map[string]interface{}{
		"enabled":       true,
		"clusters":      []interface{}{"cluster1"},
		"nameservers":   []interface{}{"10.96.0.10"},
		"searchDomains": []interface{}{"cluster.local"},
	}

	err := provider.Install(context.Background(), clientset, config)
	assert.NoError(t, err)

	// Verify ConfigMap was created
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "dns-discovery-config", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, configMap)
}

func TestDNSProvider_Uninstall(t *testing.T) {
	provider := NewDNSProvider()
	clientset := fake.NewSimpleClientset()

	// Create a ConfigMap to simulate previous installation
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dns-discovery-config",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(context.Background(), configMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.Uninstall(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify ConfigMap was deleted
	_, err = clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "dns-discovery-config", metav1.GetOptions{})
	assert.Error(t, err) // Should not exist
}

func TestDNSProvider_Status(t *testing.T) {
	provider := NewDNSProvider()
	clientset := fake.NewSimpleClientset()

	// Create a deployment to simulate running DNS discovery
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dns-discovery",
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
	assert.Equal(t, ServiceDiscoveryTypeDNS, status.Type)
	assert.Equal(t, "running", status.State)
	assert.True(t, status.Healthy)
}

func TestDNSProvider_Status_NoDeployment(t *testing.T) {
	provider := NewDNSProvider()
	clientset := fake.NewSimpleClientset()

	status, err := provider.Status(context.Background(), clientset)
	assert.NoError(t, err)
	assert.Equal(t, ServiceDiscoveryTypeDNS, status.Type)
	assert.Equal(t, "unknown", status.State)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.ErrorMessage, "failed to get DNS discovery deployment")
}

func TestDNSProvider_HealthCheck(t *testing.T) {
	provider := NewDNSProvider()
	clientset := fake.NewSimpleClientset()

	// Create a deployment to simulate running DNS discovery
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dns-discovery",
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

	assert.True(t, checkNames["dns-resolution"], "Should have dns-resolution check")
	assert.True(t, checkNames["coredns-service"], "Should have coredns-service check")
	assert.True(t, checkNames["dns-discovery-deployment"], "Should have dns-discovery-deployment check")
}

func TestDNSProvider_parseDNSConfig(t *testing.T) {
	provider := NewDNSProvider()

	config := map[string]interface{}{
		"enabled":       true,
		"clusters":      []interface{}{"cluster1", "cluster2"},
		"nameservers":   []interface{}{"10.96.0.10", "8.8.8.8"},
		"searchDomains": []interface{}{"cluster.local", "svc.cluster.local"},
		"ttl":           60,
	}

	dnsConfig, err := provider.parseDNSConfig(config)
	assert.NoError(t, err)
	assert.True(t, dnsConfig.Enabled)
	assert.Equal(t, []string{"cluster1", "cluster2"}, dnsConfig.Clusters)
	assert.Equal(t, []string{"10.96.0.10", "8.8.8.8"}, dnsConfig.Nameservers)
	assert.Equal(t, []string{"cluster.local", "svc.cluster.local"}, dnsConfig.SearchDomains)
	assert.Equal(t, 60, dnsConfig.TTL)
}

func TestDNSProvider_configureCoreDNS(t *testing.T) {
	provider := NewDNSProvider()
	clientset := fake.NewSimpleClientset()

	// Create CoreDNS ConfigMap
	corednsConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"Corefile": ".:53 {\n    kubernetes cluster.local\n    forward . 8.8.8.8\n}\n",
		},
	}
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(context.Background(), corednsConfigMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	dnsConfig := &DNSConfig{
		Clusters:      []string{"federation-cluster"},
		SearchDomains: []string{"federation.local"},
	}

	err = provider.configureCoreDNS(context.Background(), clientset, dnsConfig)
	assert.NoError(t, err)

	// Verify CoreDNS config was updated
	updatedConfigMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Contains(t, updatedConfigMap.Data["Corefile"], "federation")
}

func TestDNSProvider_createDNSDiscoveryDeployment(t *testing.T) {
	provider := NewDNSProvider()
	clientset := fake.NewSimpleClientset()

	dnsConfig := &DNSConfig{
		Enabled:  true,
		Clusters: []string{"cluster1"},
	}

	err := provider.createDNSDiscoveryDeployment(context.Background(), clientset, dnsConfig)
	assert.NoError(t, err)

	// Verify deployment was created
	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "dns-discovery", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, "dns-discovery", deployment.Name)
	assert.Equal(t, "kube-system", deployment.Namespace)
}

func TestDNSProvider_deleteDNSDiscoveryDeployment(t *testing.T) {
	provider := NewDNSProvider()
	clientset := fake.NewSimpleClientset()

	// Create a deployment first
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dns-discovery",
			Namespace: "kube-system",
		},
	}
	_, err := clientset.AppsV1().Deployments("kube-system").Create(context.Background(), deployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = provider.deleteDNSDiscoveryDeployment(context.Background(), clientset)
	assert.NoError(t, err)

	// Verify deployment was deleted
	_, err = clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "dns-discovery", metav1.GetOptions{})
	assert.Error(t, err) // Should not exist
}

func TestDNSProvider_testDNSResolution(t *testing.T) {
	provider := NewDNSProvider()

	// Test with a hostname that should fail (non-existent domain)
	err := provider.testDNSResolution("definitely-non-existent-domain-12345.invalid")
	// This should return an error since the domain doesn't exist
	assert.Error(t, err, "DNS resolution should fail for non-existent domain")
}
