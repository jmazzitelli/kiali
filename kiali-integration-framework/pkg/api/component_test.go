package api

import (
	"context"
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock Component Manager for testing
type mockComponentManager struct {
	mock.Mock
}

func (m *mockComponentManager) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockComponentManager) Type() types.ComponentType {
	args := m.Called()
	return args.Get(0).(types.ComponentType)
}

func (m *mockComponentManager) ValidateConfig(config types.ComponentConfig) error {
	args := m.Called(config)
	return args.Error(0)
}

func (m *mockComponentManager) Install(ctx context.Context, env *types.Environment, config types.ComponentConfig) error {
	args := m.Called(ctx, env, config)
	return args.Error(0)
}

func (m *mockComponentManager) Uninstall(ctx context.Context, env *types.Environment, name string) error {
	args := m.Called(ctx, env, name)
	return args.Error(0)
}

func (m *mockComponentManager) GetStatus(ctx context.Context, env *types.Environment, component *types.Component) (types.ComponentStatus, error) {
	args := m.Called(ctx, env, component)
	return args.Get(0).(types.ComponentStatus), args.Error(1)
}

func (m *mockComponentManager) Update(ctx context.Context, env *types.Environment, config types.ComponentConfig) error {
	args := m.Called(ctx, env, config)
	return args.Error(0)
}

func TestNewComponentAPI(t *testing.T) {
	api := NewComponentAPI()
	assert.NotNil(t, api)
}

func TestComponentAPI_GetSupportedComponents(t *testing.T) {
	api := NewComponentAPI()
	supported := api.GetSupportedComponents()

	assert.Contains(t, supported, types.ComponentTypeIstio)
	assert.Contains(t, supported, types.ComponentTypeKiali)
	assert.Contains(t, supported, types.ComponentTypePrometheus)
	assert.NotContains(t, supported, types.ComponentTypeJaeger)  // Not implemented yet
	assert.NotContains(t, supported, types.ComponentTypeGrafana) // Not implemented yet
}

func TestComponentAPI_IsComponentSupported(t *testing.T) {
	api := NewComponentAPI()

	assert.True(t, api.IsComponentSupported(types.ComponentTypeIstio))
	assert.True(t, api.IsComponentSupported(types.ComponentTypeKiali))
	assert.True(t, api.IsComponentSupported(types.ComponentTypePrometheus))
	assert.False(t, api.IsComponentSupported(types.ComponentTypeJaeger))
	assert.False(t, api.IsComponentSupported(types.ComponentTypeGrafana))
}

func TestComponentAPI_CreateManager(t *testing.T) {
	api := NewComponentAPI()

	// Test creating supported managers
	istioManager, err := api.CreateManager(types.ComponentTypeIstio)
	assert.NoError(t, err)
	assert.NotNil(t, istioManager)
	assert.Equal(t, types.ComponentTypeIstio, istioManager.Type())

	kialiManager, err := api.CreateManager(types.ComponentTypeKiali)
	assert.NoError(t, err)
	assert.NotNil(t, kialiManager)
	assert.Equal(t, types.ComponentTypeKiali, kialiManager.Type())

	prometheusManager, err := api.CreateManager(types.ComponentTypePrometheus)
	assert.NoError(t, err)
	assert.NotNil(t, prometheusManager)
	assert.Equal(t, types.ComponentTypePrometheus, prometheusManager.Type())

	// Test creating unsupported managers
	_, err = api.CreateManager(types.ComponentTypeJaeger)
	assert.Error(t, err)

	_, err = api.CreateManager(types.ComponentTypeGrafana)
	assert.Error(t, err)
}

func TestComponentAPI_InstallComponent(t *testing.T) {
	api := NewComponentAPI()
	ctx := context.Background()
	env := &types.Environment{}
	config := types.ComponentConfig{
		Type:    types.ComponentTypeIstio,
		Version: "1.20.0",
	}

	// This should work because Istio is supported
	err := api.InstallComponent(ctx, env, types.ComponentTypeIstio, config)
	// We expect an error because there's no cluster, but it should be a cluster error, not a manager error
	assert.Error(t, err)
}

func TestComponentAPI_UninstallComponent(t *testing.T) {
	api := NewComponentAPI()
	ctx := context.Background()
	env := &types.Environment{}

	err := api.UninstallComponent(ctx, env, types.ComponentTypeIstio, "istio")
	assert.Error(t, err) // Expect cluster connection error
}

func TestComponentAPI_GetComponentStatus(t *testing.T) {
	api := NewComponentAPI()
	ctx := context.Background()
	env := &types.Environment{}
	component := &types.Component{
		Name: "istio",
		Type: types.ComponentTypeIstio,
	}

	status, err := api.GetComponentStatus(ctx, env, types.ComponentTypeIstio, component)
	assert.Error(t, err)                                 // Expect cluster connection error
	assert.Equal(t, types.ComponentStatusFailed, status) // Status should be failed when there's an error
}

func TestComponentAPI_UpdateComponent(t *testing.T) {
	api := NewComponentAPI()
	ctx := context.Background()
	env := &types.Environment{}
	config := types.ComponentConfig{
		Type:    types.ComponentTypeIstio,
		Version: "1.20.1",
	}

	err := api.UpdateComponent(ctx, env, types.ComponentTypeIstio, config)
	assert.Error(t, err) // Expect cluster connection error
}

func TestComponentAPI_InstallComponents(t *testing.T) {
	api := NewComponentAPI()
	ctx := context.Background()
	env := &types.Environment{}

	// Test with empty components
	err := api.InstallComponents(ctx, env, map[string]types.ComponentConfig{})
	assert.NoError(t, err)

	// Test with disabled component
	components := map[string]types.ComponentConfig{
		"istio": {
			Type:    types.ComponentTypeIstio,
			Version: "1.20.0",
			Enabled: false,
		},
	}
	err = api.InstallComponents(ctx, env, components)
	assert.NoError(t, err)

	// Test with enabled component
	components["istio"] = types.ComponentConfig{
		Type:    types.ComponentTypeIstio,
		Version: "1.20.0",
		Enabled: true,
	}
	err = api.InstallComponents(ctx, env, components)
	assert.Error(t, err) // Expect cluster connection error
}

func TestComponentAPI_GetAllComponentStatuses(t *testing.T) {
	api := NewComponentAPI()
	ctx := context.Background()
	env := &types.Environment{
		Components: map[string]types.ComponentConfig{
			"istio": {
				Type:    types.ComponentTypeIstio,
				Version: "1.20.0",
				Enabled: true,
			},
		},
	}

	statuses, err := api.GetAllComponentStatuses(ctx, env)
	assert.Error(t, err)    // Expect cluster connection error
	assert.Nil(t, statuses) // Should return nil when there's an error
}
