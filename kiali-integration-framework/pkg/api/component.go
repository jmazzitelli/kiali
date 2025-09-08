package api

import (
	"context"

	"github.com/kiali/kiali-integration-framework/internal/component"
	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
)

// ComponentAPI provides a public interface for component management
type ComponentAPI interface {
	// Manager management
	CreateManager(componentType types.ComponentType) (types.ComponentManagerInterface, error)
	GetSupportedComponents() []types.ComponentType
	IsComponentSupported(componentType types.ComponentType) bool

	// Component operations (delegated to manager)
	InstallComponent(ctx context.Context, env *types.Environment, componentType types.ComponentType, config types.ComponentConfig) error
	UninstallComponent(ctx context.Context, env *types.Environment, componentType types.ComponentType, name string) error
	GetComponentStatus(ctx context.Context, env *types.Environment, componentType types.ComponentType, component *types.Component) (types.ComponentStatus, error)
	UpdateComponent(ctx context.Context, env *types.Environment, componentType types.ComponentType, config types.ComponentConfig) error

	// Bulk operations
	InstallComponents(ctx context.Context, env *types.Environment, components map[string]types.ComponentConfig) error
	GetAllComponentStatuses(ctx context.Context, env *types.Environment) (map[string]types.ComponentStatus, error)
}

// componentAPIImpl implements ComponentAPI
type componentAPIImpl struct {
	factory *component.ManagerFactory
}

// NewComponentAPI creates a new component API instance
func NewComponentAPI() ComponentAPI {
	return &componentAPIImpl{
		factory: component.NewManagerFactory(),
	}
}

// CreateManager creates a component manager
func (c *componentAPIImpl) CreateManager(componentType types.ComponentType) (types.ComponentManagerInterface, error) {
	return c.factory.CreateManager(componentType)
}

// GetSupportedComponents returns supported component types
func (c *componentAPIImpl) GetSupportedComponents() []types.ComponentType {
	return c.factory.GetSupportedComponents()
}

// IsComponentSupported checks if a component type is supported
func (c *componentAPIImpl) IsComponentSupported(componentType types.ComponentType) bool {
	return c.factory.IsComponentSupported(componentType)
}

// InstallComponent installs a component using the appropriate manager
func (c *componentAPIImpl) InstallComponent(ctx context.Context, env *types.Environment, componentType types.ComponentType, config types.ComponentConfig) error {
	manager, err := c.factory.CreateManager(componentType)
	if err != nil {
		return err
	}

	return manager.Install(ctx, env, config)
}

// UninstallComponent uninstalls a component using the appropriate manager
func (c *componentAPIImpl) UninstallComponent(ctx context.Context, env *types.Environment, componentType types.ComponentType, name string) error {
	manager, err := c.factory.CreateManager(componentType)
	if err != nil {
		return err
	}

	return manager.Uninstall(ctx, env, name)
}

// GetComponentStatus gets the status of a component using the appropriate manager
func (c *componentAPIImpl) GetComponentStatus(ctx context.Context, env *types.Environment, componentType types.ComponentType, component *types.Component) (types.ComponentStatus, error) {
	manager, err := c.factory.CreateManager(componentType)
	if err != nil {
		return types.ComponentStatusNotInstalled, err
	}

	return manager.GetStatus(ctx, env, component)
}

// UpdateComponent updates a component using the appropriate manager
func (c *componentAPIImpl) UpdateComponent(ctx context.Context, env *types.Environment, componentType types.ComponentType, config types.ComponentConfig) error {
	manager, err := c.factory.CreateManager(componentType)
	if err != nil {
		return err
	}

	return manager.Update(ctx, env, config)
}

// InstallComponents installs multiple components
func (c *componentAPIImpl) InstallComponents(ctx context.Context, env *types.Environment, components map[string]types.ComponentConfig) error {
	for componentName, config := range components {
		if !config.Enabled {
			continue
		}

		// Log which component is being installed
		if logger := utils.GetGlobalLogger(); logger != nil {
			logger.Infof("Installing component: %s (%s)", componentName, config.Type)
		}

		if err := c.InstallComponent(ctx, env, config.Type, config); err != nil {
			return err
		}
	}
	return nil
}

// GetAllComponentStatuses gets the status of all components
func (c *componentAPIImpl) GetAllComponentStatuses(ctx context.Context, env *types.Environment) (map[string]types.ComponentStatus, error) {
	statuses := make(map[string]types.ComponentStatus)

	for componentName, config := range env.Components {
		component := &types.Component{
			Name:   componentName,
			Type:   config.Type,
			Config: config,
		}

		status, err := c.GetComponentStatus(ctx, env, config.Type, component)
		if err != nil {
			return nil, err
		}

		statuses[componentName] = status
	}

	return statuses, nil
}
