package component

import (
	"fmt"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
)

// ManagerFactory creates component managers based on type
type ManagerFactory struct {
	logger *utils.Logger
}

// NewManagerFactory creates a new component manager factory
func NewManagerFactory() *ManagerFactory {
	return &ManagerFactory{
		logger: utils.GetGlobalLogger(),
	}
}

// CreateManager creates a component manager based on the component type
func (mf *ManagerFactory) CreateManager(componentType types.ComponentType) (types.ComponentManagerInterface, error) {
	mf.logger.Debugf("Creating component manager: %s", componentType)

	switch componentType {
	case types.ComponentTypeIstio:
		return NewIstioManager(), nil
	case types.ComponentTypeKiali:
		return NewKialiManager(), nil
	case types.ComponentTypePrometheus:
		return NewPrometheusManager(), nil
	case types.ComponentTypeJaeger:
		return nil, utils.NewFrameworkError(utils.ErrCodeInternalError,
			"Jaeger manager not implemented yet", nil)
	case types.ComponentTypeGrafana:
		return nil, utils.NewFrameworkError(utils.ErrCodeInternalError,
			"Grafana manager not implemented yet", nil)

	// Federation components
	case types.ComponentTypeIstioFederation:
		return NewIstioFederationManager(), nil
	case types.ComponentTypeRemoteFederation:
		return NewRemoteFederationManager(), nil
	case types.ComponentTypeGateway:
		return NewGatewayManager(), nil

	// Connectivity components
	case types.ComponentTypeNetworkConnectivity:
		return NewNetworkConnectivityManager(), nil

	default:
		return nil, utils.NewFrameworkError(utils.ErrCodeInvalidParameter,
			fmt.Sprintf("unsupported component type: %s", componentType), nil)
	}
}

// GetSupportedComponents returns a list of supported component types
func (mf *ManagerFactory) GetSupportedComponents() []types.ComponentType {
	return []types.ComponentType{
		types.ComponentTypeIstio,
		types.ComponentTypeKiali,
		types.ComponentTypePrometheus,
		// types.ComponentTypeJaeger,    // Not implemented yet
		// types.ComponentTypeGrafana,   // Not implemented yet

		// Federation components
		types.ComponentTypeIstioFederation,
		types.ComponentTypeRemoteFederation,
		types.ComponentTypeGateway,

		// Connectivity components
		types.ComponentTypeNetworkConnectivity,
	}
}

// IsComponentSupported checks if a component type is supported
func (mf *ManagerFactory) IsComponentSupported(componentType types.ComponentType) bool {
	supported := mf.GetSupportedComponents()
	for _, supportedType := range supported {
		if componentType == supportedType {
			return true
		}
	}
	return false
}
