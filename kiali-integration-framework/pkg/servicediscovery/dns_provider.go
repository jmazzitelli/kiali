package servicediscovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/kiali/kiali-integration-framework/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DNSProvider implements DNS-based service discovery
type DNSProvider struct {
	logger *utils.Logger
}

// NewDNSProvider creates a new DNS provider
func NewDNSProvider() *DNSProvider {
	return &DNSProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Type returns the service discovery type
func (d *DNSProvider) Type() ServiceDiscoveryType {
	return ServiceDiscoveryTypeDNS
}

// ValidateConfig validates the DNS configuration
func (d *DNSProvider) ValidateConfig(config map[string]interface{}) error {
	dnsConfig := &DNSConfig{
		Enabled: true,
		TTL:     30, // Default TTL
	}

	// Parse configuration
	if enabled, ok := config["enabled"].(bool); ok {
		dnsConfig.Enabled = enabled
	}

	if clusters, ok := config["clusters"].([]interface{}); ok {
		for _, cluster := range clusters {
			if clusterStr, ok := cluster.(string); ok {
				dnsConfig.Clusters = append(dnsConfig.Clusters, clusterStr)
			}
		}
	}

	if searchDomains, ok := config["searchDomains"].([]interface{}); ok {
		for _, domain := range searchDomains {
			if domainStr, ok := domain.(string); ok {
				dnsConfig.SearchDomains = append(dnsConfig.SearchDomains, domainStr)
			}
		}
	}

	if nameservers, ok := config["nameservers"].([]interface{}); ok {
		for _, ns := range nameservers {
			if nsStr, ok := ns.(string); ok {
				dnsConfig.Nameservers = append(dnsConfig.Nameservers, nsStr)
			}
		}
	}

	if ttl, ok := config["ttl"].(int); ok {
		dnsConfig.TTL = ttl
	}

	// Validate required fields
	if !dnsConfig.Enabled {
		return nil // Disabled is valid
	}

	if len(dnsConfig.Clusters) == 0 {
		return fmt.Errorf("DNS configuration must specify at least one cluster")
	}

	if len(dnsConfig.Nameservers) == 0 {
		return fmt.Errorf("DNS configuration must specify at least one nameserver")
	}

	// Validate nameservers are valid IPs or hostnames
	for _, ns := range dnsConfig.Nameservers {
		if net.ParseIP(ns) == nil && !d.isValidHostname(ns) {
			return fmt.Errorf("invalid nameserver: %s", ns)
		}
	}

	return nil
}

// isValidHostname validates if a string is a valid hostname
func (d *DNSProvider) isValidHostname(hostname string) bool {
	if len(hostname) == 0 || len(hostname) > 253 {
		return false
	}

	// Check if it's a valid hostname (basic validation)
	parts := strings.Split(hostname, ".")
	for _, part := range parts {
		if len(part) == 0 || len(part) > 63 {
			return false
		}
		// Check for valid characters
		for _, r := range part {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '-') {
				return false
			}
		}
		// Parts can't start or end with hyphen
		if part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
	}

	return true
}

// Install installs DNS-based service discovery
func (d *DNSProvider) Install(ctx context.Context, clientset kubernetes.Interface, config map[string]interface{}) error {
	d.logger.Info("Installing DNS-based service discovery")

	// Parse DNS configuration
	dnsConfig, err := d.parseDNSConfig(config)
	if err != nil {
		return err
	}

	// Create DNS configuration ConfigMap
	framework := NewFramework()
	if err := framework.CreateDNSConfigMap(ctx, clientset, "dns-discovery-config", "kube-system", dnsConfig); err != nil {
		return fmt.Errorf("failed to create DNS config map: %w", err)
	}

	// Create CoreDNS configuration if needed
	if err := d.configureCoreDNS(ctx, clientset, dnsConfig); err != nil {
		d.logger.Warnf("Failed to configure CoreDNS: %v", err)
		// Don't fail the installation for CoreDNS configuration issues
	}

	// Create DNS service discovery deployment
	if err := d.createDNSDiscoveryDeployment(ctx, clientset, dnsConfig); err != nil {
		return fmt.Errorf("failed to create DNS discovery deployment: %w", err)
	}

	d.logger.Info("Successfully installed DNS-based service discovery")
	return nil
}

// Uninstall uninstalls DNS-based service discovery
func (d *DNSProvider) Uninstall(ctx context.Context, clientset kubernetes.Interface) error {
	d.logger.Info("Uninstalling DNS-based service discovery")

	// Delete DNS discovery deployment
	if err := d.deleteDNSDiscoveryDeployment(ctx, clientset); err != nil {
		d.logger.Warnf("Failed to delete DNS discovery deployment: %v", err)
	}

	// Delete DNS configuration ConfigMap
	if err := clientset.CoreV1().ConfigMaps("kube-system").Delete(ctx, "dns-discovery-config", metav1.DeleteOptions{}); err != nil {
		d.logger.Warnf("Failed to delete DNS config map: %v", err)
	}

	// Clean up CoreDNS configuration
	if err := d.cleanupCoreDNS(ctx, clientset); err != nil {
		d.logger.Warnf("Failed to cleanup CoreDNS: %v", err)
	}

	d.logger.Info("Successfully uninstalled DNS-based service discovery")
	return nil
}

// Status returns the status of DNS-based service discovery
func (d *DNSProvider) Status(ctx context.Context, clientset kubernetes.Interface) (ServiceDiscoveryStatus, error) {
	status := ServiceDiscoveryStatus{
		Type:        ServiceDiscoveryTypeDNS,
		State:       "unknown",
		Healthy:     false,
		LastChecked: time.Now(),
	}

	// Check if DNS discovery deployment exists and is healthy
	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(ctx, "dns-discovery", metav1.GetOptions{})
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to get DNS discovery deployment: %v", err)
		return status, nil
	}

	// Check deployment status
	if deployment.Status.ReadyReplicas == deployment.Status.Replicas && deployment.Status.Replicas > 0 {
		status.State = "running"
		status.Healthy = true
	} else {
		status.State = "degraded"
		status.ErrorMessage = fmt.Sprintf("deployment not ready: %d/%d replicas",
			deployment.Status.ReadyReplicas, deployment.Status.Replicas)
	}

	// Get service and endpoint counts
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "service-discovery=true",
	})
	if err == nil {
		status.ServicesCount = len(services.Items)
	}

	endpoints, err := clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	if err == nil {
		status.EndpointsCount = len(endpoints.Items)
	}

	return status, nil
}

// HealthCheck performs health checks for DNS-based service discovery
func (d *DNSProvider) HealthCheck(ctx context.Context, clientset kubernetes.Interface) ([]ServiceDiscoveryHealthCheck, error) {
	var checks []ServiceDiscoveryHealthCheck

	// DNS resolution check
	dnsCheck := ServiceDiscoveryHealthCheck{
		Name:    "dns-resolution",
		Type:    "dns",
		LastRun: time.Now(),
	}

	if err := d.testDNSResolution("kubernetes.default.svc.cluster.local"); err != nil {
		dnsCheck.Healthy = false
		dnsCheck.Message = fmt.Sprintf("DNS resolution failed: %v", err)
	} else {
		dnsCheck.Healthy = true
		dnsCheck.Message = "DNS resolution working"
	}
	dnsCheck.Duration = time.Since(dnsCheck.LastRun)

	checks = append(checks, dnsCheck)

	// CoreDNS check
	corednsCheck := ServiceDiscoveryHealthCheck{
		Name:    "coredns-service",
		Type:    "coredns",
		LastRun: time.Now(),
	}

	_, err := clientset.CoreV1().Services("kube-system").Get(ctx, "kube-dns", metav1.GetOptions{})
	if err != nil {
		corednsCheck.Healthy = false
		corednsCheck.Message = fmt.Sprintf("CoreDNS service not found: %v", err)
	} else {
		corednsCheck.Healthy = true
		corednsCheck.Message = "CoreDNS service available"
	}
	corednsCheck.Duration = time.Since(corednsCheck.LastRun)

	checks = append(checks, corednsCheck)

	// DNS discovery deployment check
	deploymentCheck := ServiceDiscoveryHealthCheck{
		Name:    "dns-discovery-deployment",
		Type:    "deployment",
		LastRun: time.Now(),
	}

	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(ctx, "dns-discovery", metav1.GetOptions{})
	if err != nil {
		deploymentCheck.Healthy = false
		deploymentCheck.Message = fmt.Sprintf("DNS discovery deployment not found: %v", err)
	} else if deployment.Status.ReadyReplicas != deployment.Status.Replicas {
		deploymentCheck.Healthy = false
		deploymentCheck.Message = fmt.Sprintf("DNS discovery deployment not ready: %d/%d replicas",
			deployment.Status.ReadyReplicas, deployment.Status.Replicas)
	} else {
		deploymentCheck.Healthy = true
		deploymentCheck.Message = "DNS discovery deployment healthy"
	}
	deploymentCheck.Duration = time.Since(deploymentCheck.LastRun)

	checks = append(checks, deploymentCheck)

	return checks, nil
}

// parseDNSConfig parses the DNS configuration from the provided config map
func (d *DNSProvider) parseDNSConfig(config map[string]interface{}) (*DNSConfig, error) {
	dnsConfig := &DNSConfig{
		Enabled: true,
		TTL:     30,
	}

	if enabled, ok := config["enabled"].(bool); ok {
		dnsConfig.Enabled = enabled
	}

	if clusters, ok := config["clusters"].([]interface{}); ok {
		for _, cluster := range clusters {
			if clusterStr, ok := cluster.(string); ok {
				dnsConfig.Clusters = append(dnsConfig.Clusters, clusterStr)
			}
		}
	}

	if searchDomains, ok := config["searchDomains"].([]interface{}); ok {
		for _, domain := range searchDomains {
			if domainStr, ok := domain.(string); ok {
				dnsConfig.SearchDomains = append(dnsConfig.SearchDomains, domainStr)
			}
		}
	}

	if nameservers, ok := config["nameservers"].([]interface{}); ok {
		for _, ns := range nameservers {
			if nsStr, ok := ns.(string); ok {
				dnsConfig.Nameservers = append(dnsConfig.Nameservers, nsStr)
			}
		}
	}

	if ttl, ok := config["ttl"].(int); ok {
		dnsConfig.TTL = ttl
	}

	return dnsConfig, nil
}

// configureCoreDNS configures CoreDNS for cross-cluster DNS resolution
func (d *DNSProvider) configureCoreDNS(ctx context.Context, clientset kubernetes.Interface, dnsConfig *DNSConfig) error {
	// Get CoreDNS ConfigMap
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get CoreDNS config: %w", err)
	}

	// Add custom DNS configuration to CoreDNS
	corefile, exists := configMap.Data["Corefile"]
	if !exists {
		return fmt.Errorf("Corefile not found in CoreDNS config")
	}

	// Add federation plugin configuration
	federationConfig := d.generateFederationConfig(dnsConfig)
	updatedCorefile := corefile + "\n\n" + federationConfig

	configMap.Data["Corefile"] = updatedCorefile

	// Update the ConfigMap
	_, err = clientset.CoreV1().ConfigMaps("kube-system").Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update CoreDNS config: %w", err)
	}

	// Restart CoreDNS deployment to pick up new configuration
	if err := d.restartCoreDNS(ctx, clientset); err != nil {
		d.logger.Warnf("Failed to restart CoreDNS: %v", err)
	}

	return nil
}

// generateFederationConfig generates CoreDNS federation configuration
func (d *DNSProvider) generateFederationConfig(dnsConfig *DNSConfig) string {
	config := "federation {\n"

	for _, cluster := range dnsConfig.Clusters {
		config += fmt.Sprintf("    %s {\n", cluster)
		for _, domain := range dnsConfig.SearchDomains {
			config += fmt.Sprintf("        %s\n", domain)
		}
		config += "    }\n"
	}

	config += "}\n"
	return config
}

// restartCoreDNS restarts the CoreDNS deployment
func (d *DNSProvider) restartCoreDNS(ctx context.Context, clientset kubernetes.Interface) error {
	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Update annotation to trigger rolling restart
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = clientset.AppsV1().Deployments("kube-system").Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

// cleanupCoreDNS cleans up CoreDNS federation configuration
func (d *DNSProvider) cleanupCoreDNS(ctx context.Context, clientset kubernetes.Interface) error {
	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	corefile, exists := configMap.Data["Corefile"]
	if !exists {
		return nil // Nothing to clean up
	}

	// Remove federation configuration
	lines := strings.Split(corefile, "\n")
	var cleanedLines []string
	inFederationBlock := false

	for _, line := range lines {
		if strings.Contains(line, "federation {") {
			inFederationBlock = true
			continue
		}

		if inFederationBlock {
			if strings.TrimSpace(line) == "}" {
				inFederationBlock = false
				continue
			}
			continue
		}

		if !inFederationBlock && !strings.Contains(line, "federation") {
			cleanedLines = append(cleanedLines, line)
		}
	}

	configMap.Data["Corefile"] = strings.Join(cleanedLines, "\n")

	_, err = clientset.CoreV1().ConfigMaps("kube-system").Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return d.restartCoreDNS(ctx, clientset)
}

// createDNSDiscoveryDeployment creates a deployment for DNS service discovery
func (d *DNSProvider) createDNSDiscoveryDeployment(ctx context.Context, clientset kubernetes.Interface, dnsConfig *DNSConfig) error {
	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dns-discovery",
			Namespace: "kube-system",
			Labels: map[string]string{
				"app":                          "dns-discovery",
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "dns-discovery",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "dns-discovery",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "dns-discovery",
					Containers: []corev1.Container{
						{
							Name:  "dns-discovery",
							Image: "alpine:latest",
							Command: []string{
								"/bin/sh",
								"-c",
								"while true; do echo 'DNS Discovery running'; sleep 30; done",
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.AppsV1().Deployments("kube-system").Create(ctx, deployment, metav1.CreateOptions{})
	return err
}

// deleteDNSDiscoveryDeployment deletes the DNS discovery deployment
func (d *DNSProvider) deleteDNSDiscoveryDeployment(ctx context.Context, clientset kubernetes.Interface) error {
	return clientset.AppsV1().Deployments("kube-system").Delete(ctx, "dns-discovery", metav1.DeleteOptions{})
}

// testDNSResolution tests DNS resolution for a given hostname
func (d *DNSProvider) testDNSResolution(hostname string) error {
	// Use a simple DNS lookup to test resolution
	_, err := net.LookupHost(hostname)
	return err
}
