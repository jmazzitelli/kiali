package cluster

import (
	"context"
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
)

func TestMinikubeProvider_Name(t *testing.T) {
	provider := NewMinikubeProvider()
	if provider.Name() != types.ClusterProviderMinikube {
		t.Errorf("Expected provider name to be %s, got %s",
			types.ClusterProviderMinikube, provider.Name())
	}
}

func TestMinikubeProvider_CreateCluster(t *testing.T) {
	provider := NewMinikubeProvider()

	config := types.ClusterConfig{
		Provider: types.ClusterProviderMinikube,
		Name:     "test-cluster-minikube",
		Version:  "1.28.0",
		Config: map[string]interface{}{
			"memory": "2g",
			"cpus":   "1",
		},
	}

	// Note: This test requires Minikube to be installed
	// In a real CI environment, we would have Minikube available
	// For now, we'll test the error handling when Minikube is not available

	ctx := context.Background()
	err := provider.Create(ctx, config)

	// We expect this to fail in a test environment without Minikube
	// but we want to make sure it fails gracefully
	if err == nil {
		t.Log("Minikube cluster creation succeeded (Minikube is available)")

		// If it succeeded, clean up
		defer func() {
			if deleteErr := provider.Delete(ctx, config.Name); deleteErr != nil {
				t.Logf("Failed to clean up test cluster: %v", deleteErr)
			}
		}()
	} else {
		t.Logf("Minikube cluster creation failed as expected: %v", err)

		// Check that it's a proper FrameworkError
		if !utils.IsFrameworkError(err) {
			t.Errorf("Expected FrameworkError, got %T", err)
		}

		// Check error code
		if utils.GetErrorCode(err) != utils.ErrCodeClusterCreateFailed {
			t.Errorf("Expected error code %s, got %s",
				utils.ErrCodeClusterCreateFailed, utils.GetErrorCode(err))
		}
	}
}

func TestMinikubeProvider_DeleteNonExistentCluster(t *testing.T) {
	provider := NewMinikubeProvider()

	ctx := context.Background()
	err := provider.Delete(ctx, "non-existent-cluster-minikube")

	// We expect this to fail gracefully
	if err == nil {
		t.Error("Expected error when deleting non-existent cluster")
	} else {
		// Check that it's a proper FrameworkError
		if !utils.IsFrameworkError(err) {
			t.Errorf("Expected FrameworkError, got %T", err)
		}

		// Should be either cluster not found or cluster delete failed
		errorCode := utils.GetErrorCode(err)
		if errorCode != utils.ErrCodeClusterNotFound && errorCode != utils.ErrCodeClusterDeleteFailed {
			t.Errorf("Expected error code %s or %s, got %s",
				utils.ErrCodeClusterNotFound, utils.ErrCodeClusterDeleteFailed, errorCode)
		}
	}
}

func TestMinikubeProvider_StatusNonExistentCluster(t *testing.T) {
	provider := NewMinikubeProvider()

	ctx := context.Background()
	status, err := provider.Status(ctx, "non-existent-cluster-minikube")

	// Status should not return an error for non-existent clusters
	// but should indicate the cluster is not found
	if err != nil {
		t.Logf("Status returned error for non-existent cluster: %v", err)
	} else {
		if status.State != "not_found" {
			t.Errorf("Expected status state to be 'not_found', got '%s'", status.State)
		}

		if status.Healthy {
			t.Error("Expected non-existent cluster to be marked as unhealthy")
		}
	}
}

func TestMinikubeProvider_GetKubeconfigNonExistentCluster(t *testing.T) {
	provider := NewMinikubeProvider()

	ctx := context.Background()
	_, err := provider.GetKubeconfig(ctx, "non-existent-cluster-minikube")

	// We expect this to fail
	if err == nil {
		t.Error("Expected error when getting kubeconfig for non-existent cluster")
	} else {
		// Check that it's a proper FrameworkError
		if !utils.IsFrameworkError(err) {
			t.Errorf("Expected FrameworkError, got %T", err)
		}

		// Should be cluster not found
		if utils.GetErrorCode(err) != utils.ErrCodeClusterNotFound {
			t.Errorf("Expected error code %s, got %s",
				utils.ErrCodeClusterNotFound, utils.GetErrorCode(err))
		}
	}
}

func TestMinikubeProvider_ClusterExists(t *testing.T) {
	provider := NewMinikubeProvider()

	// Test with a cluster name that definitely doesn't exist
	exists, err := provider.clusterExists("definitely-non-existent-cluster-name-12345")

	// We expect this to work even without Minikube installed
	// It should return false for non-existent clusters
	if err != nil {
		t.Logf("clusterExists returned error (expected in environment without Minikube): %v", err)
		// This is acceptable - the method should handle missing Minikube gracefully
	} else {
		if exists {
			t.Errorf("Expected clusterExists to return false for non-existent cluster, got true")
		}
	}
}

func TestMinikubeProvider_ParseStatusText(t *testing.T) {
	provider := NewMinikubeProvider()

	// Test parsing running status
	runningOutput := `minikube: Running
cluster: Running
kubectl: Correctly Configured: pointing to minikube-vm:8443`

	status, err := provider.parseStatusText(runningOutput)
	if err != nil {
		t.Fatalf("Failed to parse running status: %v", err)
	}
	if status.State != "running" {
		t.Errorf("Expected state 'running', got '%s'", status.State)
	}
	if !status.Healthy {
		t.Errorf("Expected healthy=true for running cluster")
	}

	// Test parsing stopped status
	stoppedOutput := `minikube: Stopped
cluster: Stopped`

	status, err = provider.parseStatusText(stoppedOutput)
	if err != nil {
		t.Fatalf("Failed to parse stopped status: %v", err)
	}
	if status.State != "stopped" {
		t.Errorf("Expected state 'stopped', got '%s'", status.State)
	}
	if status.Healthy {
		t.Errorf("Expected healthy=false for stopped cluster")
	}
}

func TestMinikubeProvider_RunMinikubeCommand(t *testing.T) {
	provider := NewMinikubeProvider()

	ctx := context.Background()

	// Test a command that should work even without Minikube installed
	_, err := provider.runMinikubeCommand(ctx, "version", "--short")

	// We expect this to fail in an environment without Minikube
	// but we want to make sure it fails gracefully
	if err == nil {
		t.Log("Minikube command succeeded (Minikube is available)")
	} else {
		t.Logf("Minikube command failed as expected: %v", err)

		// Check that it's a proper FrameworkError
		if !utils.IsFrameworkError(err) {
			t.Errorf("Expected FrameworkError, got %T", err)
		}
	}
}

func TestMinikubeProvider_GetClusterInfo(t *testing.T) {
	provider := NewMinikubeProvider()

	// Test with invalid kubeconfig
	_, _, err := provider.getClusterInfo("invalid-kubeconfig")

	// We expect this to fail gracefully
	if err == nil {
		t.Error("Expected error when getting cluster info with invalid kubeconfig")
	}
}

// Update the factory tests to reflect that Minikube is now supported
func TestProviderFactory_CreateMinikubeProvider(t *testing.T) {
	factory := NewProviderFactory()

	// Test creating Minikube provider
	provider, err := factory.CreateProvider(types.ClusterProviderMinikube)
	if err != nil {
		t.Fatalf("Failed to create Minikube provider: %v", err)
	}

	if provider == nil {
		t.Fatal("Minikube provider should not be nil")
	}

	if provider.Name() != types.ClusterProviderMinikube {
		t.Errorf("Expected provider name to be %s, got %s",
			types.ClusterProviderMinikube, provider.Name())
	}
}

func TestProviderFactory_IsMinikubeSupported(t *testing.T) {
	factory := NewProviderFactory()

	// Test that Minikube is now supported
	if !factory.IsProviderSupported(types.ClusterProviderMinikube) {
		t.Error("Expected Minikube to be supported")
	}
}

func TestProviderFactory_GetSupportedProviders_IncludesMinikube(t *testing.T) {
	factory := NewProviderFactory()

	supported := factory.GetSupportedProviders()

	if len(supported) == 0 {
		t.Error("Expected at least one supported provider")
	}

	// Check that Minikube is supported
	found := false
	for _, provider := range supported {
		if provider == types.ClusterProviderMinikube {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected Minikube to be in supported providers list")
	}
}
