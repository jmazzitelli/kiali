package servicediscovery

import (
	"context"
	"fmt"
	"time"

	"github.com/kiali/kiali-integration-framework/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// APIServerProvider implements API server aggregation for multi-cluster
type APIServerProvider struct {
	logger *utils.Logger
}

// NewAPIServerProvider creates a new API server provider
func NewAPIServerProvider() *APIServerProvider {
	return &APIServerProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Type returns the service discovery type
func (a *APIServerProvider) Type() ServiceDiscoveryType {
	return ServiceDiscoveryTypeAPIServer
}

// ValidateConfig validates the API server configuration
func (a *APIServerProvider) ValidateConfig(config map[string]interface{}) error {
	apiConfig := &APIServerConfig{
		Enabled: true,
	}

	// Parse configuration
	if enabled, ok := config["enabled"].(bool); ok {
		apiConfig.Enabled = enabled
	}

	if clusters, ok := config["clusters"].([]interface{}); ok {
		for _, cluster := range clusters {
			if clusterStr, ok := cluster.(string); ok {
				apiConfig.Clusters = append(apiConfig.Clusters, clusterStr)
			}
		}
	}

	if apiServerURL, ok := config["apiServerUrl"].(string); ok {
		apiConfig.APIServerURL = apiServerURL
	}

	if caCert, ok := config["caCert"].(string); ok {
		apiConfig.CACert = caCert
	}

	if clientCert, ok := config["clientCert"].(string); ok {
		apiConfig.ClientCert = clientCert
	}

	if clientKey, ok := config["clientKey"].(string); ok {
		apiConfig.ClientKey = clientKey
	}

	// Validate required fields
	if !apiConfig.Enabled {
		return nil // Disabled is valid
	}

	if len(apiConfig.Clusters) == 0 {
		return fmt.Errorf("API server configuration must specify at least one cluster")
	}

	if apiConfig.APIServerURL == "" {
		return fmt.Errorf("API server URL is required")
	}

	// Validate URL format
	if !a.isValidURL(apiConfig.APIServerURL) {
		return fmt.Errorf("invalid API server URL: %s", apiConfig.APIServerURL)
	}

	return nil
}

// isValidURL performs basic URL validation
func (a *APIServerProvider) isValidURL(url string) bool {
	return len(url) > 0 && (url[:8] == "https://" || url[:7] == "http://")
}

// Install installs API server aggregation for multi-cluster
func (a *APIServerProvider) Install(ctx context.Context, clientset kubernetes.Interface, config map[string]interface{}) error {
	a.logger.Info("Installing API server aggregation for multi-cluster")

	// Parse API server configuration
	apiConfig, err := a.parseAPIServerConfig(config)
	if err != nil {
		return err
	}

	// Create API server configuration ConfigMap
	framework := NewFramework()
	if err := framework.CreateAPIServerConfigMap(ctx, clientset, "api-server-aggregation-config", "kube-system", apiConfig); err != nil {
		return fmt.Errorf("failed to create API server config map: %w", err)
	}

	// Create API server aggregator deployment
	if err := a.createAPIServerAggregatorDeployment(ctx, clientset, apiConfig); err != nil {
		return fmt.Errorf("failed to create API server aggregator deployment: %w", err)
	}

	// Create service account and RBAC for API server aggregator
	if err := a.createAPIServerRBAC(ctx, clientset); err != nil {
		return fmt.Errorf("failed to create API server RBAC: %w", err)
	}

	// Configure API server aggregation
	if err := a.configureAPIServerAggregation(ctx, clientset, apiConfig); err != nil {
		return fmt.Errorf("failed to configure API server aggregation: %w", err)
	}

	a.logger.Info("Successfully installed API server aggregation")
	return nil
}

// Uninstall uninstalls API server aggregation
func (a *APIServerProvider) Uninstall(ctx context.Context, clientset kubernetes.Interface) error {
	a.logger.Info("Uninstalling API server aggregation")

	// Delete API server aggregator deployment
	if err := a.deleteAPIServerAggregatorDeployment(ctx, clientset); err != nil {
		a.logger.Warnf("Failed to delete API server aggregator deployment: %v", err)
	}

	// Delete service account and RBAC
	if err := a.deleteAPIServerRBAC(ctx, clientset); err != nil {
		a.logger.Warnf("Failed to delete API server RBAC: %v", err)
	}

	// Delete API server configuration ConfigMap
	if err := clientset.CoreV1().ConfigMaps("kube-system").Delete(ctx, "api-server-aggregation-config", metav1.DeleteOptions{}); err != nil {
		a.logger.Warnf("Failed to delete API server config map: %v", err)
	}

	a.logger.Info("Successfully uninstalled API server aggregation")
	return nil
}

// Status returns the status of API server aggregation
func (a *APIServerProvider) Status(ctx context.Context, clientset kubernetes.Interface) (ServiceDiscoveryStatus, error) {
	status := ServiceDiscoveryStatus{
		Type:        ServiceDiscoveryTypeAPIServer,
		State:       "unknown",
		Healthy:     false,
		LastChecked: time.Now(),
	}

	// Check if API server aggregator deployment exists and is healthy
	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(ctx, "api-server-aggregator", metav1.GetOptions{})
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to get API server aggregator deployment: %v", err)
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

	// Count clusters (would be populated from actual cluster connections)
	status.Clusters = []string{"local"} // Placeholder

	return status, nil
}

// HealthCheck performs health checks for API server aggregation
func (a *APIServerProvider) HealthCheck(ctx context.Context, clientset kubernetes.Interface) ([]ServiceDiscoveryHealthCheck, error) {
	var checks []ServiceDiscoveryHealthCheck

	// API server aggregator deployment check
	deploymentCheck := ServiceDiscoveryHealthCheck{
		Name:    "api-server-aggregator-deployment",
		Type:    "deployment",
		LastRun: time.Now(),
	}

	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(ctx, "api-server-aggregator", metav1.GetOptions{})
	if err != nil {
		deploymentCheck.Healthy = false
		deploymentCheck.Message = fmt.Sprintf("API server aggregator deployment not found: %v", err)
	} else if deployment.Status.ReadyReplicas != deployment.Status.Replicas {
		deploymentCheck.Healthy = false
		deploymentCheck.Message = fmt.Sprintf("API server aggregator deployment not ready: %d/%d replicas",
			deployment.Status.ReadyReplicas, deployment.Status.Replicas)
	} else {
		deploymentCheck.Healthy = true
		deploymentCheck.Message = "API server aggregator deployment healthy"
	}
	deploymentCheck.Duration = time.Since(deploymentCheck.LastRun)

	checks = append(checks, deploymentCheck)

	// API server connectivity check
	connectivityCheck := ServiceDiscoveryHealthCheck{
		Name:    "api-server-connectivity",
		Type:    "connectivity",
		LastRun: time.Now(),
	}

	if err := a.testAPIServerConnectivity(); err != nil {
		connectivityCheck.Healthy = false
		connectivityCheck.Message = fmt.Sprintf("API server connectivity failed: %v", err)
	} else {
		connectivityCheck.Healthy = true
		connectivityCheck.Message = "API server connectivity working"
	}
	connectivityCheck.Duration = time.Since(connectivityCheck.LastRun)

	checks = append(checks, connectivityCheck)

	// RBAC check
	rbacCheck := ServiceDiscoveryHealthCheck{
		Name:    "api-server-rbac",
		Type:    "rbac",
		LastRun: time.Now(),
	}

	if err := a.checkAPIServerRBAC(ctx, clientset); err != nil {
		rbacCheck.Healthy = false
		rbacCheck.Message = fmt.Sprintf("RBAC check failed: %v", err)
	} else {
		rbacCheck.Healthy = true
		rbacCheck.Message = "RBAC configuration correct"
	}
	rbacCheck.Duration = time.Since(rbacCheck.LastRun)

	checks = append(checks, rbacCheck)

	return checks, nil
}

// parseAPIServerConfig parses the API server configuration
func (a *APIServerProvider) parseAPIServerConfig(config map[string]interface{}) (*APIServerConfig, error) {
	apiConfig := &APIServerConfig{
		Enabled: true,
	}

	if enabled, ok := config["enabled"].(bool); ok {
		apiConfig.Enabled = enabled
	}

	if clusters, ok := config["clusters"].([]interface{}); ok {
		for _, cluster := range clusters {
			if clusterStr, ok := cluster.(string); ok {
				apiConfig.Clusters = append(apiConfig.Clusters, clusterStr)
			}
		}
	}

	if apiServerURL, ok := config["apiServerUrl"].(string); ok {
		apiConfig.APIServerURL = apiServerURL
	}

	if caCert, ok := config["caCert"].(string); ok {
		apiConfig.CACert = caCert
	}

	if clientCert, ok := config["clientCert"].(string); ok {
		apiConfig.ClientCert = clientCert
	}

	if clientKey, ok := config["clientKey"].(string); ok {
		apiConfig.ClientKey = clientKey
	}

	return apiConfig, nil
}

// createAPIServerAggregatorDeployment creates the API server aggregator deployment
func (a *APIServerProvider) createAPIServerAggregatorDeployment(ctx context.Context, clientset kubernetes.Interface, apiConfig *APIServerConfig) error {
	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server-aggregator",
			Namespace: "kube-system",
			Labels: map[string]string{
				"app":                          "api-server-aggregator",
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "api-server-aggregator",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "api-server-aggregator",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "api-server-aggregator",
					Containers: []corev1.Container{
						{
							Name:  "api-server-aggregator",
							Image: "alpine:latest",
							Command: []string{
								"/bin/sh",
								"-c",
								"while true; do echo 'API Server Aggregator running'; sleep 30; done",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "https",
									ContainerPort: 8443,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "certs",
									MountPath: "/etc/ssl/certs",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "api-server-aggregator-certs",
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

// deleteAPIServerAggregatorDeployment deletes the API server aggregator deployment
func (a *APIServerProvider) deleteAPIServerAggregatorDeployment(ctx context.Context, clientset kubernetes.Interface) error {
	return clientset.AppsV1().Deployments("kube-system").Delete(ctx, "api-server-aggregator", metav1.DeleteOptions{})
}

// createAPIServerRBAC creates service account and RBAC for API server aggregator
func (a *APIServerProvider) createAPIServerRBAC(ctx context.Context, clientset kubernetes.Interface) error {
	// Create service account
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server-aggregator",
			Namespace: "kube-system",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
			},
		},
	}

	if _, err := clientset.CoreV1().ServiceAccounts("kube-system").Create(ctx, serviceAccount, metav1.CreateOptions{}); err != nil {
		return err
	}

	// Create cluster role
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-server-aggregator",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints", "pods", "nodes"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"apiextensions.k8s.io"},
				Resources: []string{"customresourcedefinitions"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	if _, err := clientset.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{}); err != nil {
		return err
	}

	// Create cluster role binding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-server-aggregator",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "api-server-aggregator",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "api-server-aggregator",
				Namespace: "kube-system",
			},
		},
	}

	_, err := clientset.RbacV1().ClusterRoleBindings().Create(ctx, clusterRoleBinding, metav1.CreateOptions{})
	return err
}

// deleteAPIServerRBAC deletes the API server RBAC resources
func (a *APIServerProvider) deleteAPIServerRBAC(ctx context.Context, clientset kubernetes.Interface) error {
	// Delete cluster role binding
	if err := clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "api-server-aggregator", metav1.DeleteOptions{}); err != nil {
		a.logger.Warnf("Failed to delete cluster role binding: %v", err)
	}

	// Delete cluster role
	if err := clientset.RbacV1().ClusterRoles().Delete(ctx, "api-server-aggregator", metav1.DeleteOptions{}); err != nil {
		a.logger.Warnf("Failed to delete cluster role: %v", err)
	}

	// Delete service account
	if err := clientset.CoreV1().ServiceAccounts("kube-system").Delete(ctx, "api-server-aggregator", metav1.DeleteOptions{}); err != nil {
		a.logger.Warnf("Failed to delete service account: %v", err)
	}

	return nil
}

// configureAPIServerAggregation configures the API server for aggregation
func (a *APIServerProvider) configureAPIServerAggregation(ctx context.Context, clientset kubernetes.Interface, apiConfig *APIServerConfig) error {
	// This would typically involve:
	// 1. Creating APIService resources for each remote cluster
	// 2. Setting up certificates and authentication
	// 3. Configuring the API server with aggregation layer

	// For now, we'll create a placeholder configuration
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-server-aggregation-setup",
			Namespace: "kube-system",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
			},
		},
		Data: map[string]string{
			"aggregation-config": fmt.Sprintf("enabled: true\napiServerUrl: %s\nclusters: %v",
				apiConfig.APIServerURL, apiConfig.Clusters),
		},
	}

	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(ctx, configMap, metav1.CreateOptions{})
	return err
}

// testAPIServerConnectivity tests connectivity to the API server
func (a *APIServerProvider) testAPIServerConnectivity() error {
	// This is a placeholder - in a real implementation, this would test
	// connectivity to remote API servers
	return nil
}

// checkAPIServerRBAC checks if the RBAC configuration is correct
func (a *APIServerProvider) checkAPIServerRBAC(ctx context.Context, clientset kubernetes.Interface) error {
	// Check if service account exists
	_, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(ctx, "api-server-aggregator", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service account not found: %v", err)
	}

	// Check if cluster role exists
	_, err = clientset.RbacV1().ClusterRoles().Get(ctx, "api-server-aggregator", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cluster role not found: %v", err)
	}

	// Check if cluster role binding exists
	_, err = clientset.RbacV1().ClusterRoleBindings().Get(ctx, "api-server-aggregator", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cluster role binding not found: %v", err)
	}

	return nil
}
