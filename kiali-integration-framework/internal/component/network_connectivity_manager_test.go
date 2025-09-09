package component

import (
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestNewNetworkConnectivityManager(t *testing.T) {
	manager := NewNetworkConnectivityManager()
	assert.NotNil(t, manager)
	assert.Equal(t, "Network Connectivity Manager", manager.Name())
	assert.Equal(t, types.ComponentTypeNetworkConnectivity, manager.Type())
}

func TestNetworkConnectivityManager_ValidateConfig_Valid(t *testing.T) {
	manager := NewNetworkConnectivityManager()

	config := types.ComponentConfig{
		Type:    types.ComponentTypeNetworkConnectivity,
		Version: "1.0.0",
		Enabled: true,
		Config: map[string]interface{}{
			"connectivity": map[string]interface{}{
				"type": "kubernetes",
			},
		},
	}

	err := manager.ValidateConfig(config)
	assert.NoError(t, err)
}

func TestNetworkConnectivityManager_ValidateConfig_InvalidType(t *testing.T) {
	manager := NewNetworkConnectivityManager()

	config := types.ComponentConfig{
		Type:    types.ComponentTypeIstio, // Wrong type
		Version: "1.0.0",
		Enabled: true,
		Config: map[string]interface{}{
			"connectivity": map[string]interface{}{
				"type": "kubernetes",
			},
		},
	}

	err := manager.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid component type")
}

func TestNetworkConnectivityManager_ValidateConfig_MissingVersion(t *testing.T) {
	manager := NewNetworkConnectivityManager()

	config := types.ComponentConfig{
		Type:    types.ComponentTypeNetworkConnectivity,
		Version: "", // Missing version
		Enabled: true,
		Config: map[string]interface{}{
			"connectivity": map[string]interface{}{
				"type": "kubernetes",
			},
		},
	}

	err := manager.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version is required")
}

func TestNetworkConnectivityManager_ValidateConfig_MissingConnectivityConfig(t *testing.T) {
	manager := NewNetworkConnectivityManager()

	config := types.ComponentConfig{
		Type:    types.ComponentTypeNetworkConnectivity,
		Version: "1.0.0",
		Enabled: true,
		Config:  map[string]interface{}{}, // Missing connectivity config
	}

	err := manager.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connectivity configuration is required")
}

func TestNetworkConnectivityManager_ValidateConfig_InvalidConnectivityType(t *testing.T) {
	manager := NewNetworkConnectivityManager()

	config := types.ComponentConfig{
		Type:    types.ComponentTypeNetworkConnectivity,
		Version: "1.0.0",
		Enabled: true,
		Config: map[string]interface{}{
			"connectivity": map[string]interface{}{
				"type": "invalid-type", // Invalid type
			},
		},
	}

	err := manager.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connectivity type must be one of")
}

func TestNetworkConnectivityManager_ValidateConfig_ValidConnectivityTypes(t *testing.T) {
	manager := NewNetworkConnectivityManager()

	validTypes := []string{"kubernetes", "istio", "linkerd", "manual"}

	for _, connectivityType := range validTypes {
		t.Run(connectivityType, func(t *testing.T) {
			config := types.ComponentConfig{
				Type:    types.ComponentTypeNetworkConnectivity,
				Version: "1.0.0",
				Enabled: true,
				Config: map[string]interface{}{
					"connectivity": map[string]interface{}{
						"type": connectivityType,
					},
				},
			}

			err := manager.ValidateConfig(config)
			assert.NoError(t, err, "Type %s should be valid", connectivityType)
		})
	}
}
