package component

import (
	"context"
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestNewIstioManager(t *testing.T) {
	manager := NewIstioManager()
	assert.NotNil(t, manager)
	assert.Equal(t, types.ComponentTypeIstio, manager.Type())
	assert.Equal(t, "Istio Manager", manager.Name())
}

func TestIstioManager_ValidateConfig(t *testing.T) {
	manager := NewIstioManager()

	tests := []struct {
		name        string
		config      types.ComponentConfig
		expectError bool
		errorCode   string
	}{
		{
			name: "valid config",
			config: types.ComponentConfig{
				Type:    types.ComponentTypeIstio,
				Version: "1.20.0",
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "wrong component type",
			config: types.ComponentConfig{
				Type:    types.ComponentTypeKiali,
				Version: "1.20.0",
			},
			expectError: true,
			errorCode:   string(utils.ErrCodeInvalidParameter),
		},
		{
			name: "missing version",
			config: types.ComponentConfig{
				Type: types.ComponentTypeIstio,
			},
			expectError: true,
			errorCode:   string(utils.ErrCodeConfigInvalid),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if ferr, ok := err.(*utils.FrameworkError); ok {
					assert.Equal(t, tt.errorCode, string(ferr.Code))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Note: GetStatus tests are simplified due to Kubernetes client mocking complexity
// In a production environment, we'd use proper mocking libraries or dependency injection

func TestIstioManager_GetStatus_NotInstalled(t *testing.T) {
	// This test verifies that GetStatus works correctly
	// Note: In a real test environment, we'd need to mock the Kubernetes client
	// For now, we test the error handling when cluster access fails
	env := &types.Environment{
		Cluster: types.ClusterConfig{
			Provider: types.ClusterProviderKind,
			Name:     "non-existent-cluster",
		},
	}

	component := &types.Component{
		Name:   "istio",
		Type:   types.ComponentTypeIstio,
		Config: types.ComponentConfig{Type: types.ComponentTypeIstio, Version: "1.20.0"},
	}

	manager := NewIstioManager()

	// This should return an error because the cluster doesn't exist
	// In a real implementation, we'd mock the client
	_, err := manager.GetStatus(context.Background(), env, component)
	assert.Error(t, err) // Expect error due to cluster access failure
}

func TestIstioManager_Install_Uninstall(t *testing.T) {
	// Test that Install and Uninstall methods exist and can be called
	// These are integration tests that would require a real cluster
	env := &types.Environment{
		Cluster: types.ClusterConfig{
			Provider: types.ClusterProviderKind,
			Name:     "test-cluster",
		},
	}

	config := types.ComponentConfig{
		Type:    types.ComponentTypeIstio,
		Version: "1.20.0",
		Enabled: true,
	}

	manager := NewIstioManager()

	// These should return errors due to no cluster access, but shouldn't panic
	err := manager.Install(context.Background(), env, config)
	assert.Error(t, err) // Expect error due to cluster access failure

	err = manager.Uninstall(context.Background(), env, "istio")
	assert.Error(t, err) // Expect error due to cluster access failure
}

func TestIstioManager_Update(t *testing.T) {
	env := &types.Environment{
		Cluster: types.ClusterConfig{
			Provider: types.ClusterProviderKind,
			Name:     "test-cluster",
		},
	}

	config := types.ComponentConfig{
		Type:    types.ComponentTypeIstio,
		Version: "1.20.1",
		Enabled: true,
	}

	manager := NewIstioManager()

	// This should return an error due to no cluster access, but shouldn't panic
	err := manager.Update(context.Background(), env, config)
	assert.Error(t, err) // Expect error due to cluster access failure
}

// Helper function to create int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}
