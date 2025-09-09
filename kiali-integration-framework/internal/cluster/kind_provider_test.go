package cluster

import (
	"context"
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
)

func TestKindProvider_Name(t *testing.T) {
	provider := NewKindProvider()
	if provider.Name() != types.ClusterProviderKind {
		t.Errorf("Expected provider name to be %s, got %s",
			types.ClusterProviderKind, provider.Name())
	}
}

func TestKindProvider_CreateCluster(t *testing.T) {
	provider := NewKindProvider()

	config := types.ClusterConfig{
		Provider: types.ClusterProviderKind,
		Name:     "test-cluster",
		Version:  "1.27.0",
		Config: map[string]interface{}{
			"nodes": 1,
		},
	}

	// Note: This test requires KinD to be installed
	// In a real CI environment, we would have KinD available
	// For now, we'll test the error handling when KinD is not available

	ctx := context.Background()
	err := provider.Create(ctx, config)

	// We expect this to fail in a test environment without KinD
	// but we want to make sure it fails gracefully
	if err == nil {
		t.Log("KinD cluster creation succeeded (KinD is available)")

		// If it succeeded, clean up
		defer func() {
			if deleteErr := provider.Delete(ctx, config.Name); deleteErr != nil {
				t.Logf("Failed to clean up test cluster: %v", deleteErr)
			}
		}()
	} else {
		t.Logf("KinD cluster creation failed as expected: %v", err)

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

func TestKindProvider_DeleteNonExistentCluster(t *testing.T) {
	provider := NewKindProvider()

	ctx := context.Background()
	err := provider.Delete(ctx, "non-existent-cluster")

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

func TestKindProvider_StatusNonExistentCluster(t *testing.T) {
	provider := NewKindProvider()

	ctx := context.Background()
	status, err := provider.Status(ctx, "non-existent-cluster")

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

func TestKindProvider_GetKubeconfigNonExistentCluster(t *testing.T) {
	provider := NewKindProvider()

	ctx := context.Background()
	_, err := provider.GetKubeconfig(ctx, "non-existent-cluster")

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

func TestProviderFactory_CreateProvider(t *testing.T) {
	factory := NewProviderFactory()

	// Test creating KinD provider
	provider, err := factory.CreateProvider(types.ClusterProviderKind)
	if err != nil {
		t.Fatalf("Failed to create KinD provider: %v", err)
	}

	if provider == nil {
		t.Fatal("KinD provider should not be nil")
	}

	if provider.Name() != types.ClusterProviderKind {
		t.Errorf("Expected provider name to be %s, got %s",
			types.ClusterProviderKind, provider.Name())
	}

	// Test creating Minikube provider (now supported)
	minikubeProvider, err := factory.CreateProvider(types.ClusterProviderMinikube)
	if err != nil {
		t.Fatalf("Failed to create Minikube provider: %v", err)
	}

	if minikubeProvider == nil {
		t.Fatal("Minikube provider should not be nil")
	}

	if minikubeProvider.Name() != types.ClusterProviderMinikube {
		t.Errorf("Expected provider name to be %s, got %s",
			types.ClusterProviderMinikube, minikubeProvider.Name())
	}

	// Test creating unsupported provider (K3s)
	_, err = factory.CreateProvider(types.ClusterProviderK3s)
	if err == nil {
		t.Error("Expected error when creating unsupported provider")
	} else {
		if !utils.IsFrameworkError(err) {
			t.Errorf("Expected FrameworkError, got %T", err)
		}

		if utils.GetErrorCode(err) != utils.ErrCodeInternalError {
			t.Errorf("Expected error code %s, got %s",
				utils.ErrCodeInternalError, utils.GetErrorCode(err))
		}
	}
}

func TestProviderFactory_GetSupportedProviders(t *testing.T) {
	factory := NewProviderFactory()

	supported := factory.GetSupportedProviders()

	if len(supported) == 0 {
		t.Error("Expected at least one supported provider")
	}

	// Check that KinD is supported
	found := false
	for _, provider := range supported {
		if provider == types.ClusterProviderKind {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected KinD to be in supported providers list")
	}
}

func TestProviderFactory_IsProviderSupported(t *testing.T) {
	factory := NewProviderFactory()

	// Test supported providers
	if !factory.IsProviderSupported(types.ClusterProviderKind) {
		t.Error("Expected KinD to be supported")
	}

	if !factory.IsProviderSupported(types.ClusterProviderMinikube) {
		t.Error("Expected Minikube to be supported")
	}

	// Test unsupported provider
	if factory.IsProviderSupported(types.ClusterProviderK3s) {
		t.Error("Expected K3s to not be supported")
	}
}
