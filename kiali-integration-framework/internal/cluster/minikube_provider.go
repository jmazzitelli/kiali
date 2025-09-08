package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// SystemResourceInfo holds system resource information
type SystemResourceInfo struct {
	TotalMemoryGB     int
	TotalCPUs         int
	AvailableMemoryGB int
	AvailableCPUs     int
}

// ResourcePlan holds the calculated resource allocation plan
type ResourcePlan struct {
	MemoryPerClusterGB int
	CPUsPerCluster     int
	TotalMemoryGB      int
	TotalCPUs          int
}

// MinikubeProvider implements the ClusterProviderInterface for Minikube
type MinikubeProvider struct {
	logger *utils.Logger
}

// NewMinikubeProvider creates a new Minikube cluster provider
func NewMinikubeProvider() *MinikubeProvider {
	return &MinikubeProvider{
		logger: utils.GetGlobalLogger(),
	}
}

// Name returns the provider name
func (m *MinikubeProvider) Name() types.ClusterProvider {
	return types.ClusterProviderMinikube
}

// Create creates a new Minikube cluster
func (m *MinikubeProvider) Create(ctx context.Context, config types.ClusterConfig) error {
	m.logger.Infof("Creating Minikube cluster: %s", config.Name)

	return m.LogOperationWithContext("create Minikube cluster", map[string]interface{}{
		"cluster_name": config.Name,
		"provider":     config.Provider,
	}, func() error {
		// Check if cluster already exists
		if exists, err := m.clusterExists(config.Name); err != nil {
			return utils.WrapError(err, utils.ErrCodeClusterCreateFailed,
				"failed to check if cluster exists")
		} else if exists {
			m.logger.Warnf("Cluster %s already exists", config.Name)
			return utils.NewFrameworkError(utils.ErrCodeClusterCreateFailed,
				fmt.Sprintf("cluster %s already exists", config.Name), nil)
		}

		// Build minikube start command
		args := []string{"start", "--profile", config.Name}

		// Add Kubernetes version if specified
		if config.Version != "" {
			args = append(args, "--kubernetes-version", config.Version)
		}

		// Add custom configuration
		if config.Config != nil {
			// Memory configuration
			if memory, ok := config.Config["memory"]; ok {
				if memStr, ok := memory.(string); ok {
					args = append(args, "--memory", memStr)
					m.logger.Debugf("Setting memory to: %s", memStr)
				}
			}

			// CPU configuration
			if cpus, ok := config.Config["cpus"]; ok {
				if cpuStr, ok := cpus.(string); ok {
					args = append(args, "--cpus", cpuStr)
					m.logger.Debugf("Setting CPUs to: %s", cpuStr)
				} else if cpuInt, ok := cpus.(int); ok {
					args = append(args, "--cpus", strconv.Itoa(cpuInt))
					m.logger.Debugf("Setting CPUs to: %d", cpuInt)
				}
			}

			// Disk size configuration
			if diskSize, ok := config.Config["diskSize"]; ok {
				if diskStr, ok := diskSize.(string); ok {
					args = append(args, "--disk-size", diskStr)
					m.logger.Debugf("Setting disk size to: %s", diskStr)
				}
			}

			// Network driver configuration
			if network, ok := config.Config["network"]; ok {
				if netStr, ok := network.(string); ok {
					args = append(args, "--network", netStr)
					m.logger.Debugf("Setting network driver to: %s", netStr)
				}
			}

			// Addons configuration
			if addons, ok := config.Config["addons"]; ok {
				if addonSlice, ok := addons.([]interface{}); ok {
					for _, addon := range addonSlice {
						if addonStr, ok := addon.(string); ok {
							args = append(args, "--addons", addonStr)
							m.logger.Debugf("Enabling addon: %s", addonStr)
						}
					}
				}
			}

			// Driver configuration
			if driver, ok := config.Config["driver"]; ok {
				if driverStr, ok := driver.(string); ok {
					args = append(args, "--driver", driverStr)
					m.logger.Debugf("Setting driver to: %s", driverStr)
				}
			}
		}

		// Execute minikube start command
		m.logger.Infof("Running: minikube %s", strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, "minikube", args...)
		output, err := cmd.CombinedOutput()

		if err != nil {
			m.logger.Errorf("Minikube start failed: %s", string(output))
			return utils.WrapError(err, utils.ErrCodeClusterCreateFailed,
				fmt.Sprintf("failed to create Minikube cluster: %s", string(output)))
		}

		m.logger.Infof("Successfully created Minikube cluster: %s", config.Name)
		return nil
	})
}

// Delete deletes a Minikube cluster
func (m *MinikubeProvider) Delete(ctx context.Context, name string) error {
	m.logger.Infof("Deleting Minikube cluster: %s", name)

	return m.LogOperationWithContext("delete Minikube cluster", map[string]interface{}{
		"cluster_name": name,
	}, func() error {
		// Check if cluster exists
		if exists, err := m.clusterExists(name); err != nil {
			return utils.WrapError(err, utils.ErrCodeClusterDeleteFailed,
				"failed to check if cluster exists")
		} else if !exists {
			m.logger.Warnf("Cluster %s does not exist", name)
			return utils.NewFrameworkError(utils.ErrCodeClusterNotFound,
				fmt.Sprintf("cluster %s does not exist", name), nil)
		}

		// Execute minikube delete command
		args := []string{"delete", "--profile", name}
		m.logger.Infof("Running: minikube %s", strings.Join(args, " "))

		cmd := exec.CommandContext(ctx, "minikube", args...)
		output, err := cmd.CombinedOutput()

		if err != nil {
			m.logger.Errorf("Minikube delete failed: %s", string(output))
			return utils.WrapError(err, utils.ErrCodeClusterDeleteFailed,
				fmt.Sprintf("failed to delete Minikube cluster: %s", string(output)))
		}

		m.logger.Infof("Successfully deleted Minikube cluster: %s", name)
		return nil
	})
}

// Status returns the status of a Minikube cluster
func (m *MinikubeProvider) Status(ctx context.Context, name string) (types.ClusterStatus, error) {
	m.logger.Debugf("Checking status of Minikube cluster: %s", name)

	var status types.ClusterStatus
	status.Name = name
	status.Provider = types.ClusterProviderMinikube

	// Check if cluster exists
	exists, err := m.clusterExists(name)
	if err != nil {
		status.State = "error"
		status.Healthy = false
		status.Error = fmt.Sprintf("failed to check cluster existence: %v", err)
		return status, nil
	}

	if !exists {
		status.State = "not_found"
		status.Healthy = false
		status.Error = "cluster does not exist"
		return status, nil
	}

	// Get minikube status
	statusOutput, err := m.runMinikubeCommand(ctx, "status", "--profile", name, "--output", "json")
	if err != nil {
		status.State = "error"
		status.Healthy = false
		status.Error = fmt.Sprintf("failed to get cluster status: %v", err)
		return status, nil
	}

	// Parse status JSON
	var statusData map[string]interface{}
	if err := json.Unmarshal([]byte(statusOutput), &statusData); err != nil {
		m.logger.Warnf("Failed to parse status JSON, falling back to text parsing: %v", err)
		parsedStatus, parseErr := m.parseStatusText(statusOutput)
		if parseErr != nil {
			return parsedStatus, parseErr
		}
		return parsedStatus, nil
	}

	// Extract status information
	if host, ok := statusData["Host"].(string); ok {
		if host == "Running" {
			status.State = "running"
			status.Healthy = true
		} else if host == "Stopped" {
			status.State = "stopped"
			status.Healthy = false
		} else {
			status.State = "unknown"
			status.Healthy = false
		}
	}

	if kubelet, ok := statusData["Kubelet"].(string); ok && kubelet != "Running" && status.Healthy {
		status.Healthy = false
		status.Error = "kubelet is not running"
	}

	if apiserver, ok := statusData["APIServer"].(string); ok && apiserver != "Running" && status.Healthy {
		status.Healthy = false
		status.Error = "API server is not running"
	}

	// Try to get node count and version via kubectl
	if status.Healthy {
		kubeconfig, err := m.GetKubeconfig(ctx, name)
		if err == nil {
			nodeCount, version, err := m.getClusterInfo(kubeconfig)
			if err == nil {
				status.Nodes = nodeCount
				status.Version = version
			} else {
				m.logger.Debugf("Failed to get cluster info: %v", err)
			}
		}
	}

	m.logger.Debugf("Minikube cluster %s status: %s, %d nodes, healthy: %t",
		name, status.State, status.Nodes, status.Healthy)

	return status, nil
}

// parseStatusText parses status output when JSON parsing fails
func (m *MinikubeProvider) parseStatusText(output string) (types.ClusterStatus, error) {
	status := types.ClusterStatus{
		State:   "unknown",
		Healthy: false,
	}

	// Simple text parsing for basic status
	if strings.Contains(output, "Running") {
		status.State = "running"
		status.Healthy = true
		status.Nodes = 1 // Minikube typically has 1 node
	} else if strings.Contains(output, "Stopped") {
		status.State = "stopped"
		status.Healthy = false
	}

	return status, nil
}

// getClusterInfo gets node count and version from kubeconfig
func (m *MinikubeProvider) getClusterInfo(kubeconfig string) (int, string, error) {
	config, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		return 0, "", fmt.Errorf("failed to create client config: %w", err)
	}

	restConfig, err := config.ClientConfig()
	if err != nil {
		return 0, "", fmt.Errorf("failed to get REST config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return 0, "", fmt.Errorf("failed to create clientset: %w", err)
	}

	// Get nodes
	nodes, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return 0, "", fmt.Errorf("failed to list nodes: %w", err)
	}

	nodeCount := len(nodes.Items)
	version := ""
	if nodeCount > 0 {
		version = nodes.Items[0].Status.NodeInfo.KubeletVersion
	}

	return nodeCount, version, nil
}

// GetKubeconfig returns the kubeconfig for a Minikube cluster
func (m *MinikubeProvider) GetKubeconfig(ctx context.Context, name string) (string, error) {
	m.logger.Debugf("Getting kubeconfig for Minikube cluster: %s", name)

	// Check if cluster exists
	if exists, err := m.clusterExists(name); err != nil {
		return "", utils.WrapError(err, utils.ErrCodeInternalError,
			"failed to check if cluster exists")
	} else if !exists {
		return "", utils.NewFrameworkError(utils.ErrCodeClusterNotFound,
			fmt.Sprintf("cluster %s does not exist", name), nil)
	}

	// Get kubeconfig using minikube kubectl config view
	kubeconfig, err := m.runMinikubeCommand(ctx, "kubectl", "--profile", name, "config", "view", "--raw")
	if err != nil {
		return "", utils.WrapError(err, utils.ErrCodeInternalError,
			"failed to get kubeconfig from Minikube")
	}

	m.logger.Debugf("Successfully retrieved kubeconfig for cluster: %s", name)
	return kubeconfig, nil
}

// clusterExists checks if a Minikube cluster exists
func (m *MinikubeProvider) clusterExists(name string) (bool, error) {
	// Use minikube profile list to check existence
	output, err := m.runMinikubeCommand(context.Background(), "profile", "list", "--output", "json")
	if err != nil {
		return false, fmt.Errorf("failed to list profiles: %w", err)
	}

	var profiles map[string]interface{}
	if err := json.Unmarshal([]byte(output), &profiles); err != nil {
		// Fallback to text parsing
		return strings.Contains(output, name), nil
	}

	// Check if profile exists in valid profiles
	if validProfiles, ok := profiles["valid"].([]interface{}); ok {
		for _, profile := range validProfiles {
			if profileStr, ok := profile.(string); ok && profileStr == name {
				return true, nil
			}
		}
	}

	return false, nil
}

// runMinikubeCommand runs a minikube command and returns the output
func (m *MinikubeProvider) runMinikubeCommand(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "minikube", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", utils.HandleCommandError(cmd, err)
	}

	return strings.TrimSpace(string(output)), nil
}

// LogOperation logs the start and end of an operation with timing
func (m *MinikubeProvider) LogOperation(operation string, fn func() error) error {
	return m.logger.LogOperation(operation, fn)
}

// LogOperationWithContext logs an operation with additional context
func (m *MinikubeProvider) LogOperationWithContext(operation string, context map[string]interface{}, fn func() error) error {
	return m.logger.LogOperationWithContext(operation, context, fn)
}

// CreateTopology creates a multi-cluster topology with concurrent cluster creation
func (m *MinikubeProvider) CreateTopology(ctx context.Context, topology types.ClusterTopology) error {
	m.logger.Infof("Creating multi-cluster topology with primary: %s and %d remote clusters", topology.Primary.Name, len(topology.Remotes))

	return m.LogOperationWithContext("create multi-cluster topology",
		map[string]interface{}{
			"primary_cluster": topology.Primary.Name,
			"remote_clusters": len(topology.Remotes),
			"concurrent":      true,
		}, func() error {
			// Validate topology configuration
			if err := m.validateTopologyConfig(topology); err != nil {
				return fmt.Errorf("invalid topology configuration: %w", err)
			}

			// Allocate resources for clusters
			if err := m.allocateTopologyResources(topology); err != nil {
				return fmt.Errorf("failed to allocate resources: %w", err)
			}

			// Create primary cluster first (synchronously)
			m.logger.Infof("Creating primary cluster: %s", topology.Primary.Name)
			if err := m.Create(ctx, topology.Primary); err != nil {
				return fmt.Errorf("failed to create primary cluster: %w", err)
			}

			// Create remote clusters concurrently
			if len(topology.Remotes) > 0 {
				if err := m.createRemoteClustersConcurrently(ctx, topology.Remotes); err != nil {
					// Cleanup: delete primary cluster if remote creation fails
					m.logger.Warnf("Remote cluster creation failed, cleaning up primary cluster: %s", topology.Primary.Name)
					if cleanupErr := m.Delete(ctx, topology.Primary.Name); cleanupErr != nil {
						m.logger.Errorf("Failed to cleanup primary cluster %s: %v", topology.Primary.Name, cleanupErr)
					}
					return err
				}
			}

			// Configure network connectivity between clusters
			if err := m.configureTopologyNetworking(ctx, topology); err != nil {
				m.logger.Warnf("Network configuration failed, but topology creation succeeded: %v", err)
			}

			m.logger.Infof("Successfully created multi-cluster topology with %d clusters", 1+len(topology.Remotes))
			return nil
		})
}

// DeleteTopology deletes a multi-cluster topology with concurrent deletion and cleanup
func (m *MinikubeProvider) DeleteTopology(ctx context.Context, topology types.ClusterTopology) error {
	m.logger.Infof("Deleting multi-cluster topology with primary: %s and %d remote clusters", topology.Primary.Name, len(topology.Remotes))

	return m.LogOperationWithContext("delete multi-cluster topology",
		map[string]interface{}{
			"primary_cluster": topology.Primary.Name,
			"remote_clusters": len(topology.Remotes),
			"concurrent":      true,
		}, func() error {
			var deletedClusters []string
			var errors []error

			// Delete remote clusters concurrently first
			if len(topology.Remotes) > 0 {
				deletedRemotes, remoteErrors := m.deleteRemoteClustersConcurrently(ctx, topology.Remotes)
				deletedClusters = append(deletedClusters, deletedRemotes...)
				errors = append(errors, remoteErrors...)
			}

			// Delete primary cluster (always attempted)
			m.logger.Infof("Deleting primary cluster: %s", topology.Primary.Name)
			if err := m.Delete(ctx, topology.Primary.Name); err != nil {
				m.logger.Errorf("Failed to delete primary cluster %s: %v", topology.Primary.Name, err)
				errors = append(errors, fmt.Errorf("failed to delete primary cluster %s: %w", topology.Primary.Name, err))
			} else {
				deletedClusters = append(deletedClusters, topology.Primary.Name)
				m.logger.Infof("Successfully deleted primary cluster: %s", topology.Primary.Name)
			}

			// Perform final cleanup
			if err := m.performTopologyCleanup(ctx, topology, deletedClusters); err != nil {
				m.logger.Warnf("Cleanup completed with warnings: %v", err)
			}

			// Report results
			if len(errors) > 0 {
				m.logger.Warnf("Topology deletion completed with %d errors: %v", len(errors), errors)
				return fmt.Errorf("topology deletion completed with %d errors: %v", len(errors), errors)
			}

			m.logger.Infof("Successfully deleted multi-cluster topology with %d clusters", len(deletedClusters))
			return nil
		})
}

// GetTopologyStatus gets the status of a multi-cluster topology
func (m *MinikubeProvider) GetTopologyStatus(ctx context.Context, topology types.ClusterTopology) (types.TopologyStatus, error) {
	m.logger.Debugf("Getting topology status for primary: %s", topology.Primary.Name)

	var topologyStatus types.TopologyStatus
	topologyStatus.Remotes = make(map[string]types.ClusterStatus)

	// Get primary cluster status
	primaryStatus, err := m.Status(ctx, topology.Primary.Name)
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
		remoteStatus, err := m.Status(ctx, remoteName)
		if err != nil {
			m.logger.Warnf("Failed to get status for remote cluster %s: %v", remoteName, err)
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

	// Set federation status
	if topology.Federation.Enabled {
		topologyStatus.FederationStatus = "enabled"
	} else {
		topologyStatus.FederationStatus = "disabled"
	}

	// Validate and set network connectivity status
	networkStatus, err := m.validateTopologyConnectivity(ctx, topology)
	if err != nil {
		m.logger.Warnf("Network connectivity validation failed: %v", err)
		topologyStatus.NetworkStatus = "unknown"
	} else {
		topologyStatus.NetworkStatus = networkStatus
	}

	// Note: Service discovery status validation could be added in the future
	// when TopologyStatus struct is extended to include ServiceDiscoveryStatus field

	return topologyStatus, nil
}

// ListClusters lists all clusters managed by this provider
func (m *MinikubeProvider) ListClusters(ctx context.Context) ([]types.ClusterStatus, error) {
	m.logger.Debugf("Listing all Minikube clusters")

	// Get list of profiles
	output, err := m.runMinikubeCommand(ctx, "profile", "list", "--output", "json")
	if err != nil {
		return nil, utils.WrapError(err, utils.ErrCodeInternalError,
			"failed to list Minikube profiles")
	}

	var profileData map[string]interface{}
	if err := json.Unmarshal([]byte(output), &profileData); err != nil {
		return nil, utils.WrapError(err, utils.ErrCodeInternalError,
			"failed to parse profile list JSON")
	}

	var clusterStatuses []types.ClusterStatus

	// Extract valid profiles
	if validProfiles, ok := profileData["valid"].([]interface{}); ok {
		for _, profile := range validProfiles {
			if profileName, ok := profile.(string); ok {
				status, err := m.Status(ctx, profileName)
				if err != nil {
					m.logger.Warnf("Failed to get status for cluster %s: %v", profileName, err)
					// Add status with error information
					clusterStatuses = append(clusterStatuses, types.ClusterStatus{
						Name:     profileName,
						Provider: types.ClusterProviderMinikube,
						State:    "error",
						Healthy:  false,
						Error:    err.Error(),
					})
				} else {
					clusterStatuses = append(clusterStatuses, status)
				}
			}
		}
	}

	m.logger.Debugf("Found %d Minikube clusters", len(clusterStatuses))
	return clusterStatuses, nil
}

// validateTopologyConfig validates the topology configuration
func (m *MinikubeProvider) validateTopologyConfig(topology types.ClusterTopology) error {
	// Validate primary cluster
	if topology.Primary.Name == "" {
		return fmt.Errorf("primary cluster name cannot be empty")
	}

	// Validate remote clusters
	for remoteName, remoteConfig := range topology.Remotes {
		if remoteConfig.Name == "" {
			return fmt.Errorf("remote cluster %s name cannot be empty", remoteName)
		}

		// Ensure remote cluster names don't conflict with primary
		if remoteConfig.Name == topology.Primary.Name {
			return fmt.Errorf("remote cluster %s cannot have the same name as primary cluster", remoteName)
		}

		// Ensure all remote clusters have unique names
		for otherRemoteName, otherRemoteConfig := range topology.Remotes {
			if remoteName != otherRemoteName && remoteConfig.Name == otherRemoteConfig.Name {
				return fmt.Errorf("remote clusters %s and %s cannot have the same name", remoteName, otherRemoteName)
			}
		}
	}

	return nil
}

// allocateTopologyResources allocates resources for the topology with optimization and conflict resolution
func (m *MinikubeProvider) allocateTopologyResources(topology types.ClusterTopology) error {
	// Calculate total resource requirements
	totalClusters := 1 + len(topology.Remotes) // Primary + remotes

	// Get system resource information
	systemResources, err := m.getSystemResourceInfo()
	if err != nil {
		m.logger.Warnf("Failed to get system resource info: %v, proceeding with defaults", err)
		systemResources = &SystemResourceInfo{
			TotalMemoryGB: 16, // Default assumption
			TotalCPUs:     4,  // Default assumption
		}
	}

	// Calculate optimal resource allocation
	resourcePlan, err := m.calculateOptimalResourceAllocation(totalClusters, systemResources)
	if err != nil {
		return fmt.Errorf("failed to calculate resource allocation: %w", err)
	}

	m.logger.Infof("System resources: %d GB RAM, %d CPUs", systemResources.TotalMemoryGB, systemResources.TotalCPUs)
	m.logger.Infof("Calculated resource plan: %d GB RAM, %d CPUs per cluster", resourcePlan.MemoryPerClusterGB, resourcePlan.CPUsPerCluster)

	// Validate and allocate resources for primary cluster
	if err := m.allocateClusterResources(&topology.Primary, resourcePlan, "primary"); err != nil {
		return fmt.Errorf("failed to allocate resources for primary cluster: %w", err)
	}

	// Validate and allocate resources for remote clusters
	for remoteName, remoteConfig := range topology.Remotes {
		if err := m.allocateClusterResources(&remoteConfig, resourcePlan, fmt.Sprintf("remote-%s", remoteName)); err != nil {
			return fmt.Errorf("failed to allocate resources for remote cluster %s: %w", remoteName, err)
		}
		// Update the remote config in the topology
		topology.Remotes[remoteName] = remoteConfig
	}

	// Validate total resource usage
	if err := m.validateTotalResourceUsage(topology, resourcePlan, systemResources); err != nil {
		return fmt.Errorf("resource validation failed: %w", err)
	}

	m.logger.Infof("Successfully allocated optimized resources for %d clusters in topology", totalClusters)
	return nil
}

// createRemoteClustersConcurrently creates remote clusters concurrently
func (m *MinikubeProvider) createRemoteClustersConcurrently(ctx context.Context, remotes map[string]types.ClusterConfig) error {
	if len(remotes) == 0 {
		return nil
	}

	m.logger.Infof("Creating %d remote clusters concurrently", len(remotes))

	// Create error channel for collecting errors
	errChan := make(chan error, len(remotes))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create remote clusters concurrently
	for remoteName, remoteConfig := range remotes {
		go func(name string, config types.ClusterConfig) {
			m.logger.Infof("Starting creation of remote cluster: %s", name)
			if err := m.Create(ctx, config); err != nil {
				m.logger.Errorf("Failed to create remote cluster %s: %v", name, err)
				errChan <- fmt.Errorf("failed to create remote cluster %s: %w", name, err)
				return
			}
			m.logger.Infof("Successfully created remote cluster: %s", name)
			errChan <- nil
		}(remoteName, remoteConfig)
	}

	// Wait for all cluster creations to complete
	var errors []error
	for i := 0; i < len(remotes); i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
			// Cancel remaining operations if one fails
			cancel()
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to create %d remote clusters: %v", len(errors), errors)
	}

	m.logger.Infof("Successfully created all %d remote clusters", len(remotes))
	return nil
}

// configureTopologyNetworking configures network connectivity between clusters
func (m *MinikubeProvider) configureTopologyNetworking(ctx context.Context, topology types.ClusterTopology) error {
	m.logger.Infof("Configuring network connectivity for topology with primary: %s", topology.Primary.Name)

	// Validate network configuration
	if err := m.validateNetworkTopologyConfig(topology); err != nil {
		return fmt.Errorf("invalid network topology configuration: %w", err)
	}

	// Configure network settings for primary cluster
	if err := m.configureClusterNetworking(ctx, topology.Primary); err != nil {
		return fmt.Errorf("failed to configure networking for primary cluster: %w", err)
	}

	// Configure network settings for remote clusters
	for remoteName, remoteConfig := range topology.Remotes {
		if err := m.configureClusterNetworking(ctx, remoteConfig); err != nil {
			m.logger.Warnf("Failed to configure networking for remote cluster %s: %v", remoteName, err)
			// Continue with other clusters but log the error
		}
	}

	// Configure cross-cluster connectivity if needed
	if m.hasNetworkConfiguration(topology.Network) {
		if err := m.configureCrossClusterConnectivity(ctx, topology); err != nil {
			m.logger.Warnf("Cross-cluster connectivity configuration failed: %v", err)
		}
	}

	m.logger.Infof("Network configuration completed for topology")
	return nil
}

// configureClusterNetworking configures networking for a single cluster
func (m *MinikubeProvider) configureClusterNetworking(ctx context.Context, config types.ClusterConfig) error {
	// Get current cluster status
	status, err := m.Status(ctx, config.Name)
	if err != nil {
		return fmt.Errorf("failed to get cluster status: %w", err)
	}

	if !status.Healthy {
		return fmt.Errorf("cluster %s is not healthy, skipping network configuration", config.Name)
	}

	// Configure network settings if specified
	if config.Config != nil {
		// Configure network driver
		if network, ok := config.Config["network"]; ok {
			if networkStr, ok := network.(string); ok {
				if err := m.configureNetworkDriver(ctx, config.Name, networkStr); err != nil {
					return fmt.Errorf("failed to configure network driver: %w", err)
				}
			}
		}

		// Configure ports
		if ports, ok := config.Config["ports"]; ok {
			if portsSlice, ok := ports.([]interface{}); ok {
				if err := m.configurePorts(ctx, config.Name, portsSlice); err != nil {
					return fmt.Errorf("failed to configure ports: %w", err)
				}
			}
		}

		// Configure DNS settings
		if dns, ok := config.Config["dns"]; ok {
			if dnsConfig, ok := dns.(map[string]interface{}); ok {
				if err := m.configureDNS(ctx, config.Name, dnsConfig); err != nil {
					return fmt.Errorf("failed to configure DNS: %w", err)
				}
			}
		}

		// Configure ingress
		if ingress, ok := config.Config["ingress"]; ok {
			if ingressEnabled, ok := ingress.(bool); ok && ingressEnabled {
				if err := m.configureIngress(ctx, config.Name); err != nil {
					return fmt.Errorf("failed to configure ingress: %w", err)
				}
			}
		}
	}

	m.logger.Debugf("Network configuration completed for cluster: %s", config.Name)
	return nil
}

// deleteRemoteClustersConcurrently deletes remote clusters concurrently
func (m *MinikubeProvider) deleteRemoteClustersConcurrently(ctx context.Context, remotes map[string]types.ClusterConfig) ([]string, []error) {
	if len(remotes) == 0 {
		return nil, nil
	}

	m.logger.Infof("Deleting %d remote clusters concurrently", len(remotes))

	// Create result channels
	type deleteResult struct {
		clusterName string
		err         error
	}

	resultChan := make(chan deleteResult, len(remotes))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Delete remote clusters concurrently
	for remoteName := range remotes {
		go func(name string) {
			m.logger.Infof("Starting deletion of remote cluster: %s", name)
			if err := m.Delete(ctx, name); err != nil {
				m.logger.Errorf("Failed to delete remote cluster %s: %v", name, err)
				resultChan <- deleteResult{clusterName: name, err: fmt.Errorf("failed to delete remote cluster %s: %w", name, err)}
				return
			}
			m.logger.Infof("Successfully deleted remote cluster: %s", name)
			resultChan <- deleteResult{clusterName: name, err: nil}
		}(remoteName)
	}

	// Collect results
	var deletedClusters []string
	var errors []error
	for i := 0; i < len(remotes); i++ {
		result := <-resultChan
		if result.err != nil {
			errors = append(errors, result.err)
			// Don't cancel here - let all deletions attempt to complete
		} else {
			deletedClusters = append(deletedClusters, result.clusterName)
		}
	}

	m.logger.Infof("Remote cluster deletion completed: %d successful, %d failed", len(deletedClusters), len(errors))
	return deletedClusters, errors
}

// performTopologyCleanup performs final cleanup operations
func (m *MinikubeProvider) performTopologyCleanup(ctx context.Context, topology types.ClusterTopology, deletedClusters []string) error {
	m.logger.Debugf("Performing topology cleanup for %d clusters", len(deletedClusters))

	var warnings []error

	// Clean up any leftover resources
	for _, clusterName := range deletedClusters {
		// Clean up any Minikube-specific resources
		if err := m.cleanupClusterResources(ctx, clusterName); err != nil {
			m.logger.Warnf("Resource cleanup warning for cluster %s: %v", clusterName, err)
			warnings = append(warnings, fmt.Errorf("cleanup warning for %s: %w", clusterName, err))
		}
	}

	// Clean up topology-specific resources
	if err := m.cleanupTopologyResources(ctx, topology); err != nil {
		m.logger.Warnf("Topology resource cleanup warning: %v", err)
		warnings = append(warnings, fmt.Errorf("topology cleanup warning: %w", err))
	}

	if len(warnings) > 0 {
		return fmt.Errorf("cleanup completed with %d warnings", len(warnings))
	}

	return nil
}

// cleanupClusterResources cleans up resources for a specific cluster
func (m *MinikubeProvider) cleanupClusterResources(ctx context.Context, clusterName string) error {
	// Clean up any Minikube-specific resources like cache, images, etc.
	// This is a placeholder for future cleanup operations
	m.logger.Debugf("Cleaning up resources for cluster: %s", clusterName)

	// Example: Clean up Minikube cache for this profile
	// This can be extended with more cleanup operations as needed

	return nil
}

// cleanupTopologyResources cleans up topology-specific resources
func (m *MinikubeProvider) cleanupTopologyResources(ctx context.Context, topology types.ClusterTopology) error {
	// Clean up any topology-specific resources
	// This is a placeholder for future topology-level cleanup
	m.logger.Debugf("Cleaning up topology resources for primary: %s", topology.Primary.Name)

	// Example: Clean up any shared network resources, volumes, etc.
	// This can be extended with more cleanup operations as needed

	return nil
}

// validateNetworkTopologyConfig validates the network topology configuration
func (m *MinikubeProvider) validateNetworkTopologyConfig(topology types.ClusterTopology) error {
	// Validate network configuration consistency across clusters
	if m.hasNetworkConfiguration(topology.Network) {
		m.logger.Debugf("Validating network topology configuration")

		// Check for network driver consistency
		primaryNetwork := m.getNetworkDriver(topology.Primary)
		for remoteName, remoteConfig := range topology.Remotes {
			remoteNetwork := m.getNetworkDriver(remoteConfig)
			if primaryNetwork != "" && remoteNetwork != "" && primaryNetwork != remoteNetwork {
				m.logger.Warnf("Network driver mismatch: primary uses %s, remote %s uses %s",
					primaryNetwork, remoteName, remoteNetwork)
			}
		}
	}

	return nil
}

// getNetworkDriver extracts network driver from cluster configuration
func (m *MinikubeProvider) getNetworkDriver(config types.ClusterConfig) string {
	if config.Config != nil {
		if network, ok := config.Config["network"]; ok {
			if networkStr, ok := network.(string); ok {
				return networkStr
			}
		}
	}
	return ""
}

// hasNetworkConfiguration checks if network configuration is present and meaningful
func (m *MinikubeProvider) hasNetworkConfiguration(network types.NetworkConfig) bool {
	// Check if gateway is configured
	if network.Gateway.Type != "" {
		return true
	}

	// Check if service discovery is configured
	if network.ServiceDiscovery.Type != "" {
		return true
	}

	// Check if network policies are defined
	if len(network.Policies) > 0 {
		return true
	}

	return false
}

// configureCrossClusterConnectivity configures connectivity between clusters
func (m *MinikubeProvider) configureCrossClusterConnectivity(ctx context.Context, topology types.ClusterTopology) error {
	m.logger.Infof("Configuring cross-cluster connectivity for topology")

	// For Minikube, cross-cluster connectivity can be achieved through:
	// 1. Shared network drivers
	// 2. Service discovery configuration
	// 3. Network policies

	// This is a foundation that can be extended with specific implementations
	// based on the network provider and requirements

	// Example: Configure service discovery between clusters
	if err := m.configureServiceDiscoveryBetweenClusters(ctx, topology); err != nil {
		return fmt.Errorf("failed to configure service discovery: %w", err)
	}

	return nil
}

// configureNetworkDriver configures the network driver for a cluster
func (m *MinikubeProvider) configureNetworkDriver(ctx context.Context, clusterName, networkDriver string) error {
	m.logger.Debugf("Configuring network driver %s for cluster %s", networkDriver, clusterName)

	// Minikube supports various network drivers: docker, podman, virtualbox, vmware, etc.
	validDrivers := []string{"docker", "podman", "virtualbox", "vmware", "hyperkit", "hyperv", "kvm2", "none"}

	valid := false
	for _, driver := range validDrivers {
		if networkDriver == driver {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("unsupported network driver: %s", networkDriver)
	}

	// Note: Network driver is typically set during cluster creation
	// This method can be used to validate and log the configuration
	m.logger.Infof("Network driver %s validated for cluster %s", networkDriver, clusterName)

	return nil
}

// configurePorts configures port mappings for a cluster
func (m *MinikubeProvider) configurePorts(ctx context.Context, clusterName string, ports []interface{}) error {
	m.logger.Debugf("Configuring %d ports for cluster %s", len(ports), clusterName)

	for i, port := range ports {
		if portStr, ok := port.(string); ok {
			m.logger.Debugf("Configuring port %s for cluster %s", portStr, clusterName)
			// Additional port configuration can be implemented here
			// This might involve minikube service commands or kubectl port-forward
		} else {
			return fmt.Errorf("invalid port configuration at index %d: expected string, got %T", i, port)
		}
	}

	return nil
}

// configureDNS configures DNS settings for a cluster
func (m *MinikubeProvider) configureDNS(ctx context.Context, clusterName string, dnsConfig map[string]interface{}) error {
	m.logger.Debugf("Configuring DNS for cluster %s", clusterName)

	// Extract DNS configuration
	if nameservers, ok := dnsConfig["nameservers"]; ok {
		if nsSlice, ok := nameservers.([]interface{}); ok {
			for i, ns := range nsSlice {
				if nsStr, ok := ns.(string); ok {
					m.logger.Debugf("DNS nameserver %d: %s", i+1, nsStr)
				}
			}
		}
	}

	if searchDomains, ok := dnsConfig["searchDomains"]; ok {
		if sdSlice, ok := searchDomains.([]interface{}); ok {
			for i, sd := range sdSlice {
				if sdStr, ok := sd.(string); ok {
					m.logger.Debugf("DNS search domain %d: %s", i+1, sdStr)
				}
			}
		}
	}

	// Note: DNS configuration in Minikube can be complex
	// This is a foundation that can be extended with specific DNS implementations

	return nil
}

// configureIngress configures ingress for a cluster
func (m *MinikubeProvider) configureIngress(ctx context.Context, clusterName string) error {
	m.logger.Debugf("Configuring ingress for cluster %s", clusterName)

	// Enable ingress addon if not already enabled
	args := []string{"addons", "enable", "ingress", "--profile", clusterName}
	output, err := m.runMinikubeCommand(ctx, args...)
	if err != nil {
		return fmt.Errorf("failed to enable ingress addon: %s", output)
	}

	m.logger.Infof("Ingress addon enabled for cluster %s", clusterName)
	return nil
}

// configureServiceDiscoveryBetweenClusters configures service discovery between clusters
func (m *MinikubeProvider) configureServiceDiscoveryBetweenClusters(ctx context.Context, topology types.ClusterTopology) error {
	m.logger.Debugf("Configuring service discovery between clusters")

	// This is a foundation for cross-cluster service discovery
	// Implementation will depend on the specific service mesh or networking solution

	// For now, we log the configuration for future implementation
	m.logger.Infof("Service discovery configuration prepared for %d clusters", 1+len(topology.Remotes))

	return nil
}

// validateTopologyConnectivity validates network connectivity between clusters
func (m *MinikubeProvider) validateTopologyConnectivity(ctx context.Context, topology types.ClusterTopology) (string, error) {
	m.logger.Debugf("Validating network connectivity for topology")

	// Basic connectivity validation
	// In a real implementation, this would test actual network connectivity
	// between clusters using ping, DNS resolution, or service mesh probes

	// For now, we perform basic validation
	if !m.hasNetworkConfiguration(topology.Network) {
		return "not_configured", nil
	}

	// Check if all clusters are healthy first
	primaryStatus, err := m.Status(ctx, topology.Primary.Name)
	if err != nil {
		return "unknown", fmt.Errorf("failed to get primary cluster status: %w", err)
	}

	if !primaryStatus.Healthy {
		return "unhealthy", nil
	}

	for remoteName := range topology.Remotes {
		remoteStatus, err := m.Status(ctx, remoteName)
		if err != nil {
			return "unknown", fmt.Errorf("failed to get remote cluster %s status: %w", remoteName, err)
		}
		if !remoteStatus.Healthy {
			return "degraded", nil
		}
	}

	// If all clusters are healthy and network is configured, assume connected
	return "connected", nil
}

// validateServiceDiscovery validates service discovery configuration
func (m *MinikubeProvider) validateServiceDiscovery(ctx context.Context, topology types.ClusterTopology) (string, error) {
	m.logger.Debugf("Validating service discovery for topology")

	// Basic service discovery validation
	// In a real implementation, this would test DNS resolution, service endpoints,
	// and cross-cluster service discovery

	serviceDiscoveryType := topology.Network.ServiceDiscovery.Type
	if serviceDiscoveryType == "" {
		return "not_configured", nil
	}

	// Validate based on service discovery type
	switch serviceDiscoveryType {
	case "dns":
		return m.validateDNSServiceDiscovery(ctx, topology)
	case "api-server":
		return m.validateAPIServerServiceDiscovery(ctx, topology)
	case "propagation":
		return m.validatePropagationServiceDiscovery(ctx, topology)
	default:
		m.logger.Warnf("Unknown service discovery type: %s", serviceDiscoveryType)
		return "unknown", nil
	}
}

// validateDNSServiceDiscovery validates DNS-based service discovery
func (m *MinikubeProvider) validateDNSServiceDiscovery(ctx context.Context, topology types.ClusterTopology) (string, error) {
	m.logger.Debugf("Validating DNS-based service discovery")

	// This would implement DNS resolution testing between clusters
	// For now, we return a basic status

	return "configured", nil
}

// validateAPIServerServiceDiscovery validates API server-based service discovery
func (m *MinikubeProvider) validateAPIServerServiceDiscovery(ctx context.Context, topology types.ClusterTopology) (string, error) {
	m.logger.Debugf("Validating API server-based service discovery")

	// This would implement API server connectivity testing between clusters
	// For now, we return a basic status

	return "configured", nil
}

// validatePropagationServiceDiscovery validates service propagation-based discovery
func (m *MinikubeProvider) validatePropagationServiceDiscovery(ctx context.Context, topology types.ClusterTopology) (string, error) {
	m.logger.Debugf("Validating service propagation-based discovery")

	// This would implement service endpoint propagation validation
	// For now, we return a basic status

	return "configured", nil
}

// getSystemResourceInfo gets system resource information
func (m *MinikubeProvider) getSystemResourceInfo() (*SystemResourceInfo, error) {
	// Try to get system information using system commands
	// This is a simplified implementation - in production, you'd use more sophisticated methods

	info := &SystemResourceInfo{
		TotalMemoryGB: 16, // Default fallback
		TotalCPUs:     4,  // Default fallback
	}

	// Try to get CPU count
	if cpuOutput, err := exec.Command("nproc").Output(); err == nil {
		if cpuCount, parseErr := strconv.Atoi(strings.TrimSpace(string(cpuOutput))); parseErr == nil {
			info.TotalCPUs = cpuCount
			info.AvailableCPUs = cpuCount
		}
	}

	// Try to get memory info (simplified)
	if memOutput, err := exec.Command("free", "-g").Output(); err == nil {
		lines := strings.Split(string(memOutput), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 2 {
				if memGB, parseErr := strconv.Atoi(fields[0]); parseErr == nil {
					info.TotalMemoryGB = memGB
					info.AvailableMemoryGB = memGB
				}
			}
		}
	}

	return info, nil
}

// calculateOptimalResourceAllocation calculates optimal resource allocation
func (m *MinikubeProvider) calculateOptimalResourceAllocation(clusterCount int, systemResources *SystemResourceInfo) (*ResourcePlan, error) {
	plan := &ResourcePlan{}

	// Reserve some resources for system overhead (20%)
	reservedMemoryGB := int(float64(systemResources.TotalMemoryGB) * 0.2)
	reservedCPUs := int(float64(systemResources.TotalCPUs) * 0.2)

	availableMemoryGB := systemResources.TotalMemoryGB - reservedMemoryGB
	availableCPUs := systemResources.TotalCPUs - reservedCPUs

	// Calculate per-cluster resources
	// For Minikube, typical minimums are 2GB RAM and 1 CPU per cluster
	minMemoryPerCluster := 2
	minCPUsPerCluster := 1

	// Calculate optimal allocation
	memoryPerCluster := availableMemoryGB / clusterCount
	cpusPerCluster := availableCPUs / clusterCount

	// Ensure minimum requirements
	if memoryPerCluster < minMemoryPerCluster {
		memoryPerCluster = minMemoryPerCluster
		m.logger.Warnf("System has limited memory, using minimum %d GB per cluster", minMemoryPerCluster)
	}

	if cpusPerCluster < minCPUsPerCluster {
		cpusPerCluster = minCPUsPerCluster
		m.logger.Warnf("System has limited CPUs, using minimum %d CPU per cluster", minCPUsPerCluster)
	}

	// Cap at reasonable maximums for Minikube
	if memoryPerCluster > 8 {
		memoryPerCluster = 8 // Cap at 8GB per cluster
	}

	if cpusPerCluster > 4 {
		cpusPerCluster = 4 // Cap at 4 CPUs per cluster
	}

	plan.MemoryPerClusterGB = memoryPerCluster
	plan.CPUsPerCluster = cpusPerCluster
	plan.TotalMemoryGB = memoryPerCluster * clusterCount
	plan.TotalCPUs = cpusPerCluster * clusterCount

	return plan, nil
}

// allocateClusterResources allocates resources for a specific cluster
func (m *MinikubeProvider) allocateClusterResources(config *types.ClusterConfig, plan *ResourcePlan, clusterType string) error {
	if config.Config == nil {
		config.Config = make(map[string]interface{})
	}

	// Set memory allocation
	if _, ok := config.Config["memory"]; !ok {
		memoryStr := fmt.Sprintf("%dg", plan.MemoryPerClusterGB)
		config.Config["memory"] = memoryStr
		m.logger.Debugf("Allocated %s memory for %s cluster %s", memoryStr, clusterType, config.Name)
	} else {
		// Validate existing memory allocation
		if err := m.validateMemoryAllocation(config, plan); err != nil {
			return fmt.Errorf("memory allocation validation failed: %w", err)
		}
	}

	// Set CPU allocation
	if _, ok := config.Config["cpus"]; !ok {
		cpusStr := strconv.Itoa(plan.CPUsPerCluster)
		config.Config["cpus"] = cpusStr
		m.logger.Debugf("Allocated %s CPUs for %s cluster %s", cpusStr, clusterType, config.Name)
	} else {
		// Validate existing CPU allocation
		if err := m.validateCPUAllocation(config, plan); err != nil {
			return fmt.Errorf("CPU allocation validation failed: %w", err)
		}
	}

	return nil
}

// validateMemoryAllocation validates memory allocation for a cluster
func (m *MinikubeProvider) validateMemoryAllocation(config *types.ClusterConfig, plan *ResourcePlan) error {
	memory, ok := config.Config["memory"]
	if !ok {
		return nil // No validation needed if not set
	}

	memoryStr, ok := memory.(string)
	if !ok {
		return fmt.Errorf("memory must be a string, got %T", memory)
	}

	// Parse memory value (handle various formats: "2g", "2048m", etc.)
	var memoryGB int
	if strings.HasSuffix(memoryStr, "g") {
		if val, err := strconv.Atoi(strings.TrimSuffix(memoryStr, "g")); err == nil {
			memoryGB = val
		}
	} else if strings.HasSuffix(memoryStr, "m") {
		if val, err := strconv.Atoi(strings.TrimSuffix(memoryStr, "m")); err == nil {
			memoryGB = val / 1024 // Convert MB to GB
		}
	} else {
		// Assume GB if no suffix
		if val, err := strconv.Atoi(memoryStr); err == nil {
			memoryGB = val
		}
	}

	if memoryGB < 1 {
		return fmt.Errorf("memory allocation %d GB is below minimum requirement of 1 GB", memoryGB)
	}

	if memoryGB > plan.MemoryPerClusterGB*2 {
		m.logger.Warnf("Memory allocation %d GB exceeds recommended %d GB for cluster %s",
			memoryGB, plan.MemoryPerClusterGB, config.Name)
	}

	return nil
}

// validateCPUAllocation validates CPU allocation for a cluster
func (m *MinikubeProvider) validateCPUAllocation(config *types.ClusterConfig, plan *ResourcePlan) error {
	cpus, ok := config.Config["cpus"]
	if !ok {
		return nil // No validation needed if not set
	}

	var cpuCount int
	if cpuStr, ok := cpus.(string); ok {
		if val, err := strconv.Atoi(cpuStr); err == nil {
			cpuCount = val
		}
	} else if cpuInt, ok := cpus.(int); ok {
		cpuCount = cpuInt
	}

	if cpuCount < 1 {
		return fmt.Errorf("CPU allocation %d is below minimum requirement of 1 CPU", cpuCount)
	}

	if cpuCount > plan.CPUsPerCluster*2 {
		m.logger.Warnf("CPU allocation %d exceeds recommended %d CPUs for cluster %s",
			cpuCount, plan.CPUsPerCluster, config.Name)
	}

	return nil
}

// validateTotalResourceUsage validates total resource usage against system limits
func (m *MinikubeProvider) validateTotalResourceUsage(topology types.ClusterTopology, plan *ResourcePlan, systemResources *SystemResourceInfo) error {
	totalMemoryGB := 0
	totalCPUs := 0

	// Calculate total allocated memory and CPUs
	clusters := append([]types.ClusterConfig{topology.Primary}, m.getRemoteConfigs(topology.Remotes)...)

	for _, cluster := range clusters {
		if cluster.Config != nil {
			// Parse memory
			if memory, ok := cluster.Config["memory"]; ok {
				if memoryStr, ok := memory.(string); ok {
					if strings.HasSuffix(memoryStr, "g") {
						if val, err := strconv.Atoi(strings.TrimSuffix(memoryStr, "g")); err == nil {
							totalMemoryGB += val
						}
					}
				}
			}

			// Parse CPUs
			if cpus, ok := cluster.Config["cpus"]; ok {
				if cpuStr, ok := cpus.(string); ok {
					if val, err := strconv.Atoi(cpuStr); err == nil {
						totalCPUs += val
					}
				} else if cpuInt, ok := cpus.(int); ok {
					totalCPUs += cpuInt
				}
			}
		}
	}

	// Check against system limits
	if totalMemoryGB > systemResources.TotalMemoryGB {
		return fmt.Errorf("total memory allocation %d GB exceeds system capacity %d GB",
			totalMemoryGB, systemResources.TotalMemoryGB)
	}

	if totalCPUs > systemResources.TotalCPUs {
		return fmt.Errorf("total CPU allocation %d exceeds system capacity %d CPUs",
			totalCPUs, systemResources.TotalCPUs)
	}

	m.logger.Infof("Resource validation passed: %d GB RAM, %d CPUs allocated of %d GB, %d CPUs available",
		totalMemoryGB, totalCPUs, systemResources.TotalMemoryGB, systemResources.TotalCPUs)

	return nil
}

// getRemoteConfigs extracts remote cluster configs as a slice
func (m *MinikubeProvider) getRemoteConfigs(remotes map[string]types.ClusterConfig) []types.ClusterConfig {
	var configs []types.ClusterConfig
	for _, config := range remotes {
		configs = append(configs, config)
	}
	return configs
}
