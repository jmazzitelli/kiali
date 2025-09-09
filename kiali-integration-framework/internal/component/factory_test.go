package component

import (
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestNewManagerFactory(t *testing.T) {
	factory := NewManagerFactory()
	assert.NotNil(t, factory)
	assert.NotNil(t, factory.logger)
}

func TestManagerFactory_GetSupportedComponents(t *testing.T) {
	factory := NewManagerFactory()
	supported := factory.GetSupportedComponents()

	assert.Contains(t, supported, types.ComponentTypeIstio)
	assert.Contains(t, supported, types.ComponentTypeKiali)
	assert.Contains(t, supported, types.ComponentTypePrometheus)
	assert.NotContains(t, supported, types.ComponentTypeJaeger)  // Not implemented yet
	assert.NotContains(t, supported, types.ComponentTypeGrafana) // Not implemented yet
}

func TestManagerFactory_IsComponentSupported(t *testing.T) {
	factory := NewManagerFactory()

	assert.True(t, factory.IsComponentSupported(types.ComponentTypeIstio))
	assert.True(t, factory.IsComponentSupported(types.ComponentTypeKiali))
	assert.True(t, factory.IsComponentSupported(types.ComponentTypePrometheus))
	assert.False(t, factory.IsComponentSupported(types.ComponentTypeJaeger))
	assert.False(t, factory.IsComponentSupported(types.ComponentTypeGrafana))
}

func TestManagerFactory_CreateManager(t *testing.T) {
	factory := NewManagerFactory()

	// Test creating Istio manager
	istioManager, err := factory.CreateManager(types.ComponentTypeIstio)
	assert.NoError(t, err)
	assert.NotNil(t, istioManager)
	assert.Equal(t, types.ComponentTypeIstio, istioManager.Type())
	assert.Equal(t, "Istio Manager", istioManager.Name())

	// Test creating Kiali manager
	kialiManager, err := factory.CreateManager(types.ComponentTypeKiali)
	assert.NoError(t, err)
	assert.NotNil(t, kialiManager)
	assert.Equal(t, types.ComponentTypeKiali, kialiManager.Type())
	assert.Equal(t, "Kiali Manager", kialiManager.Name())

	// Test creating Prometheus manager
	prometheusManager, err := factory.CreateManager(types.ComponentTypePrometheus)
	assert.NoError(t, err)
	assert.NotNil(t, prometheusManager)
	assert.Equal(t, types.ComponentTypePrometheus, prometheusManager.Type())
	assert.Equal(t, "Prometheus Manager", prometheusManager.Name())

	// Test creating unsupported managers
	_, err = factory.CreateManager(types.ComponentTypeJaeger)
	assert.Error(t, err)
	if ferr, ok := err.(*utils.FrameworkError); ok {
		assert.Equal(t, utils.ErrCodeInternalError, ferr.Code)
	}

	_, err = factory.CreateManager(types.ComponentTypeGrafana)
	assert.Error(t, err)
	if ferr, ok := err.(*utils.FrameworkError); ok {
		assert.Equal(t, utils.ErrCodeInternalError, ferr.Code)
	}

	// Test invalid component type
	_, err = factory.CreateManager("invalid-type")
	assert.Error(t, err)
	if ferr, ok := err.(*utils.FrameworkError); ok {
		assert.Equal(t, utils.ErrCodeInvalidParameter, ferr.Code)
	}
}

func TestManagerFactory_ManagerInstances(t *testing.T) {
	factory := NewManagerFactory()

	// Test that we get different instances for each manager type
	istio1, _ := factory.CreateManager(types.ComponentTypeIstio)
	istio2, _ := factory.CreateManager(types.ComponentTypeIstio)

	// They should be different instances but same type
	assert.NotSame(t, istio1, istio2)
	assert.Equal(t, istio1.Type(), istio2.Type())

	kiali1, _ := factory.CreateManager(types.ComponentTypeKiali)
	kiali2, _ := factory.CreateManager(types.ComponentTypeKiali)

	assert.NotSame(t, kiali1, kiali2)
	assert.Equal(t, kiali1.Type(), kiali2.Type())

	prometheus1, _ := factory.CreateManager(types.ComponentTypePrometheus)
	prometheus2, _ := factory.CreateManager(types.ComponentTypePrometheus)

	assert.NotSame(t, prometheus1, prometheus2)
	assert.Equal(t, prometheus1.Type(), prometheus2.Type())
}
