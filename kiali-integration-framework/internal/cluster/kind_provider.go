package cluster

import (
	"context"
	"fmt"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	v1alpha4 "sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	kindcluster "sigs.k8s.io/kind/pkg/cluster"
)

// KindProvider implements the ClusterProviderInterface for KinD
type KindProvider struct {
	logger *utils.Logger
}

// NewKindProvider creates a new KinD cluster provider
func NewKindProvider() *KindProvider {
	return &KindProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Name returns the provider name
func (k *KindProvider) Name() types.ClusterProvider {
	return types.ClusterProviderKind
}

// Create creates a new KinD cluster
func (k *KindProvider) Create(ctx context.Context, config types.ClusterConfig) error {
	k.logger.Infof("Creating KinD cluster: %s", config.Name)

	return k.LogOperationWithContext("create KinD cluster", map[string]interface{}{
		"cluster_name": config.Name,
		"provider":     config.Provider,
	}, func() error {
		// Create the KinD cluster provider
		provider := kindcluster.NewProvider()

		// Check if cluster already exists
		clusters, err := provider.List()
		if err != nil {
			return utils.WrapError(err, utils.ErrCodeClusterCreateFailed,
				"failed to list existing clusters")
		}

		for _, clusterName := range clusters {
			if clusterName == config.Name {
				k.logger.Warnf("Cluster %s already exists", config.Name)
				return utils.NewFrameworkError(utils.ErrCodeClusterCreateFailed,
					fmt.Sprintf("cluster %s already exists", config.Name), nil)
			}
		}

		// Prepare cluster configuration
		kindConfig := &v1alpha4.Cluster{
			Nodes: []v1alpha4.Node{
				{
					Role:  v1alpha4.ControlPlaneRole,
					Image: fmt.Sprintf("kindest/node:v%s", config.Version),
				},
			},
		}

		// Check for custom configuration
		if config.Config != nil {
			// Parse node count
			if nodeCount, ok := config.Config["nodes"]; ok {
				if count, ok := nodeCount.(int); ok && count > 1 {
					// Add worker nodes
					for i := 1; i < count; i++ {
						kindConfig.Nodes = append(kindConfig.Nodes, v1alpha4.Node{
							Role:  "worker",
							Image: fmt.Sprintf("kindest/node:v%s", config.Version),
						})
					}
				}
			}

			// Parse memory and CPU settings
			if memory, ok := config.Config["memory"]; ok {
				if memStr, ok := memory.(string); ok {
					k.logger.Debugf("Memory setting specified: %s", memStr)
				}
			}

			if cpus, ok := config.Config["cpus"]; ok {
				if cpuStr, ok := cpus.(string); ok {
					k.logger.Debugf("CPU setting specified: %s", cpuStr)
				}
			}
		}

		k.logger.Infof("Creating KinD cluster with %d nodes", len(kindConfig.Nodes))

		// Create the cluster
		if err := provider.Create(config.Name, kindcluster.CreateWithV1Alpha4Config(kindConfig)); err != nil {
			return utils.WrapError(err, utils.ErrCodeClusterCreateFailed,
				"failed to create KinD cluster")
		}

		k.logger.Infof("Successfully created KinD cluster: %s", config.Name)
		return nil
	})
}

// Delete deletes a KinD cluster
func (k *KindProvider) Delete(ctx context.Context, name string) error {
	k.logger.Infof("Deleting KinD cluster: %s", name)

	return k.LogOperationWithContext("delete KinD cluster", map[string]interface{}{
		"cluster_name": name,
	}, func() error {
		provider := kindcluster.NewProvider()

		// Check if cluster exists
		clusters, err := provider.List()
		if err != nil {
			return utils.WrapError(err, utils.ErrCodeClusterDeleteFailed,
				"failed to list existing clusters")
		}

		exists := false
		for _, clusterName := range clusters {
			if clusterName == name {
				exists = true
				break
			}
		}

		if !exists {
			k.logger.Warnf("Cluster %s does not exist", name)
			return utils.NewFrameworkError(utils.ErrCodeClusterNotFound,
				fmt.Sprintf("cluster %s does not exist", name), nil)
		}

		// Delete the cluster
		if err := provider.Delete(name, ""); err != nil {
			return utils.WrapError(err, utils.ErrCodeClusterDeleteFailed,
				"failed to delete KinD cluster")
		}

		k.logger.Infof("Successfully deleted KinD cluster: %s", name)
		return nil
	})
}

// Status returns the status of a KinD cluster
func (k *KindProvider) Status(ctx context.Context, name string) (types.ClusterStatus, error) {
	k.logger.Debugf("Checking status of KinD cluster: %s", name)

	var status types.ClusterStatus
	status.Name = name
	status.Provider = types.ClusterProviderKind

	// Execute status check operation
	provider := kindcluster.NewProvider()

	// Check if cluster exists
	clusters, err := provider.List()
	if err != nil {
		status.State = "error"
		status.Healthy = false
		status.Error = fmt.Sprintf("failed to list clusters: %v", err)
		return status, nil
	}

	exists := false
	for _, clusterName := range clusters {
		if clusterName == name {
			exists = true
			break
		}
	}

	if !exists {
		status.State = "not_found"
		status.Healthy = false
		status.Error = "cluster does not exist"
		return status, nil
	}

	// Get kubeconfig to check cluster health
	kubeconfig, err := provider.KubeConfig(name, false)
	if err != nil {
		status.State = "error"
		status.Healthy = false
		status.Error = fmt.Sprintf("failed to get kubeconfig: %v", err)
		return status, nil
	}

	// Try to connect to the cluster
	config, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		status.State = "error"
		status.Healthy = false
		status.Error = fmt.Sprintf("failed to create client config: %v", err)
		return status, nil
	}

	restConfig, err := config.ClientConfig()
	if err != nil {
		status.State = "error"
		status.Healthy = false
		status.Error = fmt.Sprintf("failed to get REST config: %v", err)
		return status, nil
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		status.State = "error"
		status.Healthy = false
		status.Error = fmt.Sprintf("failed to create clientset: %v", err)
		return status, nil
	}

	// Check cluster health by getting nodes
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		status.State = "unhealthy"
		status.Healthy = false
		status.Error = fmt.Sprintf("failed to list nodes: %v", err)
		return status, nil
	}

	status.State = "running"
	status.Nodes = len(nodes.Items)
	status.Healthy = true

	// Get version from first node
	if len(nodes.Items) > 0 {
		status.Version = nodes.Items[0].Status.NodeInfo.KubeletVersion
	}

	k.logger.Debugf("KinD cluster %s status: %s, %d nodes, healthy: %t",
		name, status.State, status.Nodes, status.Healthy)

	return status, nil
}

// GetKubeconfig returns the kubeconfig for a KinD cluster
func (k *KindProvider) GetKubeconfig(ctx context.Context, name string) (string, error) {
	k.logger.Debugf("Getting kubeconfig for KinD cluster: %s", name)

	// Execute kubeconfig retrieval operation
	provider := kindcluster.NewProvider()

	// Check if cluster exists
	clusters, err := provider.List()
	if err != nil {
		return "", utils.WrapError(err, utils.ErrCodeInternalError,
			"failed to list clusters")
	}

	exists := false
	for _, clusterName := range clusters {
		if clusterName == name {
			exists = true
			break
		}
	}

	if !exists {
		return "", utils.NewFrameworkError(utils.ErrCodeClusterNotFound,
			fmt.Sprintf("cluster %s does not exist", name), nil)
	}

	// Get kubeconfig
	kubeconfig, err := provider.KubeConfig(name, false)
	if err != nil {
		return "", utils.WrapError(err, utils.ErrCodeInternalError,
			"failed to get kubeconfig from KinD")
	}

	k.logger.Debugf("Successfully retrieved kubeconfig for cluster: %s", name)
	return kubeconfig, nil
}

// LogOperation logs the start and end of an operation with timing
func (k *KindProvider) LogOperation(operation string, fn func() error) error {
	return k.logger.LogOperation(operation, fn)
}

// LogOperationWithContext logs an operation with additional context
func (k *KindProvider) LogOperationWithContext(operation string, context map[string]interface{}, fn func() error) error {
	return k.logger.LogOperationWithContext(operation, context, fn)
}

// CreateTopology creates a multi-cluster topology
func (k *KindProvider) CreateTopology(ctx context.Context, topology types.ClusterTopology) error {
	k.logger.Infof("Creating multi-cluster topology with primary: %s", topology.Primary.Name)

	return k.LogOperationWithContext("create multi-cluster topology",
		map[string]interface{}{
			"primary_cluster": topology.Primary.Name,
			"remote_clusters": len(topology.Remotes),
		}, func() error {
			// Create primary cluster first
			if err := k.Create(ctx, topology.Primary); err != nil {
				return fmt.Errorf("failed to create primary cluster: %w", err)
			}

			// Create remote clusters
			for remoteName, remoteConfig := range topology.Remotes {
				k.logger.Infof("Creating remote cluster: %s", remoteName)
				if err := k.Create(ctx, remoteConfig); err != nil {
					return fmt.Errorf("failed to create remote cluster %s: %w", remoteName, err)
				}
			}

			k.logger.Infof("Successfully created multi-cluster topology")
			return nil
		})
}

// DeleteTopology deletes a multi-cluster topology
func (k *KindProvider) DeleteTopology(ctx context.Context, topology types.ClusterTopology) error {
	k.logger.Infof("Deleting multi-cluster topology with primary: %s", topology.Primary.Name)

	return k.LogOperationWithContext("delete multi-cluster topology",
		map[string]interface{}{
			"primary_cluster": topology.Primary.Name,
			"remote_clusters": len(topology.Remotes),
		}, func() error {
			// Delete remote clusters first
			for remoteName := range topology.Remotes {
				k.logger.Infof("Deleting remote cluster: %s", remoteName)
				if err := k.Delete(ctx, remoteName); err != nil {
					k.logger.Warnf("Failed to delete remote cluster %s: %v", remoteName, err)
					// Continue with other deletions
				}
			}

			// Delete primary cluster
			if err := k.Delete(ctx, topology.Primary.Name); err != nil {
				return fmt.Errorf("failed to delete primary cluster: %w", err)
			}

			k.logger.Infof("Successfully deleted multi-cluster topology")
			return nil
		})
}

// GetTopologyStatus gets the status of a multi-cluster topology
func (k *KindProvider) GetTopologyStatus(ctx context.Context, topology types.ClusterTopology) (types.TopologyStatus, error) {
	k.logger.Debugf("Getting topology status for primary: %s", topology.Primary.Name)

	var topologyStatus types.TopologyStatus
	topologyStatus.Remotes = make(map[string]types.ClusterStatus)

	// Get primary cluster status
	primaryStatus, err := k.Status(ctx, topology.Primary.Name)
	if err != nil {
		topologyStatus.Error = fmt.Sprintf("failed to get primary cluster status: %v", err)
		topologyStatus.OverallHealth = "unhealthy"
		return topologyStatus, nil
	}
	topologyStatus.Primary = primaryStatus

	// Get remote cluster statuses
	healthyCount := 0
	totalCount := 1 // Start with primary

	if primaryStatus.Healthy {
		healthyCount++
	}

	for remoteName := range topology.Remotes {
		remoteStatus, err := k.Status(ctx, remoteName)
		if err != nil {
			k.logger.Warnf("Failed to get status for remote cluster %s: %v", remoteName, err)
			remoteStatus = types.ClusterStatus{
				Name:    remoteName,
				State:   "error",
				Healthy: false,
				Error:   err.Error(),
			}
		}
		topologyStatus.Remotes[remoteName] = remoteStatus
		totalCount++

		if remoteStatus.Healthy {
			healthyCount++
		}
	}

	// Determine overall health
	switch {
	case healthyCount == totalCount:
		topologyStatus.OverallHealth = "healthy"
	case healthyCount > 0:
		topologyStatus.OverallHealth = "degraded"
	default:
		topologyStatus.OverallHealth = "unhealthy"
	}

	// Set federation status (placeholder for now)
	if topology.Federation.Enabled {
		topologyStatus.FederationStatus = "enabled"
	} else {
		topologyStatus.FederationStatus = "disabled"
	}

	// Set network status (placeholder for now)
	topologyStatus.NetworkStatus = "connected"

	return topologyStatus, nil
}

// ListClusters lists all clusters managed by this provider
func (k *KindProvider) ListClusters(ctx context.Context) ([]types.ClusterStatus, error) {
	k.logger.Debugf("Listing all KinD clusters")

	provider := kindcluster.NewProvider()

	// Get list of cluster names
	clusters, err := provider.List()
	if err != nil {
		return nil, utils.WrapError(err, utils.ErrCodeInternalError,
			"failed to list KinD clusters")
	}

	var clusterStatuses []types.ClusterStatus
	for _, clusterName := range clusters {
		status, err := k.Status(ctx, clusterName)
		if err != nil {
			k.logger.Warnf("Failed to get status for cluster %s: %v", clusterName, err)
			// Add status with error information
			clusterStatuses = append(clusterStatuses, types.ClusterStatus{
				Name:     clusterName,
				Provider: types.ClusterProviderKind,
				State:    "error",
				Healthy:  false,
				Error:    err.Error(),
			})
		} else {
			clusterStatuses = append(clusterStatuses, status)
		}
	}

	k.logger.Debugf("Found %d KinD clusters", len(clusterStatuses))
	return clusterStatuses, nil
}
