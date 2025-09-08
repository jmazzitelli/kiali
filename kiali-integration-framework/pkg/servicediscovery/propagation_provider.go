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

// PropagationProvider implements service endpoint propagation across clusters
type PropagationProvider struct {
	logger *utils.Logger
}

// NewPropagationProvider creates a new propagation provider
func NewPropagationProvider() *PropagationProvider {
	return &PropagationProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Type returns the service discovery type
func (p *PropagationProvider) Type() ServiceDiscoveryType {
	return ServiceDiscoveryTypePropagation
}

// ValidateConfig validates the service propagation configuration
func (p *PropagationProvider) ValidateConfig(config map[string]interface{}) error {
	propagationConfig := &ServicePropagationConfig{
		Enabled:      true,
		SyncInterval: time.Minute * 5, // Default 5 minutes
	}

	// Parse configuration
	if enabled, ok := config["enabled"].(bool); ok {
		propagationConfig.Enabled = enabled
	}

	if clusters, ok := config["clusters"].([]interface{}); ok {
		for _, cluster := range clusters {
			if clusterStr, ok := cluster.(string); ok {
				propagationConfig.Clusters = append(propagationConfig.Clusters, clusterStr)
			}
		}
	}

	if selectorLabels, ok := config["selectorLabels"].(map[string]interface{}); ok {
		propagationConfig.SelectorLabels = make(map[string]string)
		for k, v := range selectorLabels {
			if vStr, ok := v.(string); ok {
				propagationConfig.SelectorLabels[k] = vStr
			}
		}
	}

	if excludeLabels, ok := config["excludeLabels"].(map[string]interface{}); ok {
		propagationConfig.ExcludeLabels = make(map[string]string)
		for k, v := range excludeLabels {
			if vStr, ok := v.(string); ok {
				propagationConfig.ExcludeLabels[k] = vStr
			}
		}
	}

	if namespaces, ok := config["namespaces"].([]interface{}); ok {
		for _, ns := range namespaces {
			if nsStr, ok := ns.(string); ok {
				propagationConfig.Namespaces = append(propagationConfig.Namespaces, nsStr)
			}
		}
	}

	if syncInterval, ok := config["syncInterval"].(string); ok {
		if duration, err := time.ParseDuration(syncInterval); err == nil {
			propagationConfig.SyncInterval = duration
		}
	}

	// Validate required fields
	if !propagationConfig.Enabled {
		return nil // Disabled is valid
	}

	if len(propagationConfig.Clusters) == 0 {
		return fmt.Errorf("service propagation configuration must specify at least one cluster")
	}

	return nil
}

// Install installs service endpoint propagation
func (p *PropagationProvider) Install(ctx context.Context, clientset kubernetes.Interface, config map[string]interface{}) error {
	p.logger.Info("Installing service endpoint propagation")

	// Parse propagation configuration
	propagationConfig, err := p.parsePropagationConfig(config)
	if err != nil {
		return err
	}

	// Create propagation configuration ConfigMap
	framework := NewFramework()
	if err := framework.CreateServicePropagationConfigMap(ctx, clientset, "service-propagation-config", "kube-system", propagationConfig); err != nil {
		return fmt.Errorf("failed to create service propagation config map: %w", err)
	}

	// Create service propagator deployment
	if err := p.createServicePropagatorDeployment(ctx, clientset, propagationConfig); err != nil {
		return fmt.Errorf("failed to create service propagator deployment: %w", err)
	}

	// Create service account and RBAC for service propagator
	if err := p.createServicePropagatorRBAC(ctx, clientset); err != nil {
		return fmt.Errorf("failed to create service propagator RBAC: %w", err)
	}

	// Initialize propagation state
	if err := p.initializePropagationState(ctx, clientset); err != nil {
		return fmt.Errorf("failed to initialize propagation state: %w", err)
	}

	p.logger.Info("Successfully installed service endpoint propagation")
	return nil
}

// Uninstall uninstalls service endpoint propagation
func (p *PropagationProvider) Uninstall(ctx context.Context, clientset kubernetes.Interface) error {
	p.logger.Info("Uninstalling service endpoint propagation")

	// Delete service propagator deployment
	if err := p.deleteServicePropagatorDeployment(ctx, clientset); err != nil {
		p.logger.Warnf("Failed to delete service propagator deployment: %v", err)
	}

	// Delete service account and RBAC
	if err := p.deleteServicePropagatorRBAC(ctx, clientset); err != nil {
		p.logger.Warnf("Failed to delete service propagator RBAC: %v", err)
	}

	// Delete propagation configuration ConfigMap
	if err := clientset.CoreV1().ConfigMaps("kube-system").Delete(ctx, "service-propagation-config", metav1.DeleteOptions{}); err != nil {
		p.logger.Warnf("Failed to delete service propagation config map: %v", err)
	}

	// Clean up propagated services
	if err := p.cleanupPropagatedServices(ctx, clientset); err != nil {
		p.logger.Warnf("Failed to cleanup propagated services: %v", err)
	}

	p.logger.Info("Successfully uninstalled service endpoint propagation")
	return nil
}

// Status returns the status of service endpoint propagation
func (p *PropagationProvider) Status(ctx context.Context, clientset kubernetes.Interface) (ServiceDiscoveryStatus, error) {
	status := ServiceDiscoveryStatus{
		Type:        ServiceDiscoveryTypePropagation,
		State:       "unknown",
		Healthy:     false,
		LastChecked: time.Now(),
	}

	// Check if service propagator deployment exists and is healthy
	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(ctx, "service-propagator", metav1.GetOptions{})
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to get service propagator deployment: %v", err)
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

	// Get service and endpoint counts for propagated services
	labelSelector := "service-discovery/propagated=true"
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err == nil {
		status.ServicesCount = len(services.Items)
	}

	endpoints, err := clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err == nil {
		status.EndpointsCount = len(endpoints.Items)
	}

	return status, nil
}

// HealthCheck performs health checks for service endpoint propagation
func (p *PropagationProvider) HealthCheck(ctx context.Context, clientset kubernetes.Interface) ([]ServiceDiscoveryHealthCheck, error) {
	var checks []ServiceDiscoveryHealthCheck

	// Service propagator deployment check
	deploymentCheck := ServiceDiscoveryHealthCheck{
		Name:    "service-propagator-deployment",
		Type:    "deployment",
		LastRun: time.Now(),
	}

	deployment, err := clientset.AppsV1().Deployments("kube-system").Get(ctx, "service-propagator", metav1.GetOptions{})
	if err != nil {
		deploymentCheck.Healthy = false
		deploymentCheck.Message = fmt.Sprintf("Service propagator deployment not found: %v", err)
	} else if deployment.Status.ReadyReplicas != deployment.Status.Replicas {
		deploymentCheck.Healthy = false
		deploymentCheck.Message = fmt.Sprintf("Service propagator deployment not ready: %d/%d replicas",
			deployment.Status.ReadyReplicas, deployment.Status.Replicas)
	} else {
		deploymentCheck.Healthy = true
		deploymentCheck.Message = "Service propagator deployment healthy"
	}
	deploymentCheck.Duration = time.Since(deploymentCheck.LastRun)

	checks = append(checks, deploymentCheck)

	// Propagation state check
	stateCheck := ServiceDiscoveryHealthCheck{
		Name:    "propagation-state",
		Type:    "state",
		LastRun: time.Now(),
	}

	if err := p.checkPropagationState(ctx, clientset); err != nil {
		stateCheck.Healthy = false
		stateCheck.Message = fmt.Sprintf("Propagation state check failed: %v", err)
	} else {
		stateCheck.Healthy = true
		stateCheck.Message = "Propagation state is healthy"
	}
	stateCheck.Duration = time.Since(stateCheck.LastRun)

	checks = append(checks, stateCheck)

	// Service propagation sync check
	syncCheck := ServiceDiscoveryHealthCheck{
		Name:    "service-propagation-sync",
		Type:    "sync",
		LastRun: time.Now(),
	}

	if err := p.checkServicePropagationSync(ctx, clientset); err != nil {
		syncCheck.Healthy = false
		syncCheck.Message = fmt.Sprintf("Service propagation sync failed: %v", err)
	} else {
		syncCheck.Healthy = true
		syncCheck.Message = "Service propagation sync working"
	}
	syncCheck.Duration = time.Since(syncCheck.LastRun)

	checks = append(checks, syncCheck)

	return checks, nil
}

// parsePropagationConfig parses the service propagation configuration
func (p *PropagationProvider) parsePropagationConfig(config map[string]interface{}) (*ServicePropagationConfig, error) {
	propagationConfig := &ServicePropagationConfig{
		Enabled:      true,
		SyncInterval: time.Minute * 5,
	}

	if enabled, ok := config["enabled"].(bool); ok {
		propagationConfig.Enabled = enabled
	}

	if clusters, ok := config["clusters"].([]interface{}); ok {
		for _, cluster := range clusters {
			if clusterStr, ok := cluster.(string); ok {
				propagationConfig.Clusters = append(propagationConfig.Clusters, clusterStr)
			}
		}
	}

	if selectorLabels, ok := config["selectorLabels"].(map[string]interface{}); ok {
		propagationConfig.SelectorLabels = make(map[string]string)
		for k, v := range selectorLabels {
			if vStr, ok := v.(string); ok {
				propagationConfig.SelectorLabels[k] = vStr
			}
		}
	}

	if excludeLabels, ok := config["excludeLabels"].(map[string]interface{}); ok {
		propagationConfig.ExcludeLabels = make(map[string]string)
		for k, v := range excludeLabels {
			if vStr, ok := v.(string); ok {
				propagationConfig.ExcludeLabels[k] = vStr
			}
		}
	}

	if namespaces, ok := config["namespaces"].([]interface{}); ok {
		for _, ns := range namespaces {
			if nsStr, ok := ns.(string); ok {
				propagationConfig.Namespaces = append(propagationConfig.Namespaces, nsStr)
			}
		}
	}

	if syncInterval, ok := config["syncInterval"].(string); ok {
		if duration, err := time.ParseDuration(syncInterval); err == nil {
			propagationConfig.SyncInterval = duration
		}
	}

	return propagationConfig, nil
}

// createServicePropagatorDeployment creates the service propagator deployment
func (p *PropagationProvider) createServicePropagatorDeployment(ctx context.Context, clientset kubernetes.Interface, propagationConfig *ServicePropagationConfig) error {
	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagator",
			Namespace: "kube-system",
			Labels: map[string]string{
				"app":                          "service-propagator",
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "service-propagator",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "service-propagator",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "service-propagator",
					Containers: []corev1.Container{
						{
							Name:  "service-propagator",
							Image: "alpine:latest",
							Command: []string{
								"/bin/sh",
								"-c",
								"while true; do echo 'Service Propagator running'; sleep 30; done",
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

// deleteServicePropagatorDeployment deletes the service propagator deployment
func (p *PropagationProvider) deleteServicePropagatorDeployment(ctx context.Context, clientset kubernetes.Interface) error {
	return clientset.AppsV1().Deployments("kube-system").Delete(ctx, "service-propagator", metav1.DeleteOptions{})
}

// createServicePropagatorRBAC creates service account and RBAC for service propagator
func (p *PropagationProvider) createServicePropagatorRBAC(ctx context.Context, clientset kubernetes.Interface) error {
	// Create service account
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagator",
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
			Name: "service-propagator",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints", "configmaps"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
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
			Name: "service-propagator",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "service-propagator",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "service-propagator",
				Namespace: "kube-system",
			},
		},
	}

	_, err := clientset.RbacV1().ClusterRoleBindings().Create(ctx, clusterRoleBinding, metav1.CreateOptions{})
	return err
}

// deleteServicePropagatorRBAC deletes the service propagator RBAC resources
func (p *PropagationProvider) deleteServicePropagatorRBAC(ctx context.Context, clientset kubernetes.Interface) error {
	// Delete cluster role binding
	if err := clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "service-propagator", metav1.DeleteOptions{}); err != nil {
		p.logger.Warnf("Failed to delete cluster role binding: %v", err)
	}

	// Delete cluster role
	if err := clientset.RbacV1().ClusterRoles().Delete(ctx, "service-propagator", metav1.DeleteOptions{}); err != nil {
		p.logger.Warnf("Failed to delete cluster role: %v", err)
	}

	// Delete service account
	if err := clientset.CoreV1().ServiceAccounts("kube-system").Delete(ctx, "service-propagator", metav1.DeleteOptions{}); err != nil {
		p.logger.Warnf("Failed to delete service account: %v", err)
	}

	return nil
}

// initializePropagationState initializes the propagation state
func (p *PropagationProvider) initializePropagationState(ctx context.Context, clientset kubernetes.Interface) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-propagation-state",
			Namespace: "kube-system",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kiali-integration-framework",
				"service-discovery":            "propagation-state",
			},
		},
		Data: map[string]string{
			"last-sync": time.Now().Format(time.RFC3339),
			"services":  "0",
			"endpoints": "0",
			"status":    "initialized",
		},
	}

	_, err := clientset.CoreV1().ConfigMaps("kube-system").Create(ctx, configMap, metav1.CreateOptions{})
	return err
}

// cleanupPropagatedServices cleans up all propagated services
func (p *PropagationProvider) cleanupPropagatedServices(ctx context.Context, clientset kubernetes.Interface) error {
	labelSelector := "service-discovery/propagated=true"

	// Delete propagated services
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return err
	}

	for _, svc := range services.Items {
		if err := clientset.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{}); err != nil {
			p.logger.Warnf("Failed to delete propagated service %s/%s: %v", svc.Namespace, svc.Name, err)
		}
	}

	// Delete propagated endpoints
	endpoints, err := clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return err
	}

	for _, ep := range endpoints.Items {
		if err := clientset.CoreV1().Endpoints(ep.Namespace).Delete(ctx, ep.Name, metav1.DeleteOptions{}); err != nil {
			p.logger.Warnf("Failed to delete propagated endpoints %s/%s: %v", ep.Namespace, ep.Name, err)
		}
	}

	return nil
}

// checkPropagationState checks the propagation state
func (p *PropagationProvider) checkPropagationState(ctx context.Context, clientset kubernetes.Interface) error {
	_, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "service-propagation-state", metav1.GetOptions{})
	return err
}

// checkServicePropagationSync checks if service propagation is working
func (p *PropagationProvider) checkServicePropagationSync(ctx context.Context, clientset kubernetes.Interface) error {
	// Check if there are any propagated services
	labelSelector := "service-discovery/propagated=true"
	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return err
	}

	if len(services.Items) == 0 {
		return fmt.Errorf("no propagated services found")
	}

	return nil
}
