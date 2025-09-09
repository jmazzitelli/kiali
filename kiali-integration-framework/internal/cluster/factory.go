package cluster

import (
	"fmt"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
)

// ProviderFactory creates cluster providers based on type
type ProviderFactory struct {
	logger *utils.Logger
}

// NewProviderFactory creates a new provider factory
func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{
		logger: utils.GetGlobalLogger(),
	}
}

// CreateProvider creates a cluster provider based on the provider type
func (pf *ProviderFactory) CreateProvider(providerType types.ClusterProvider) (types.ClusterProviderInterface, error) {
	pf.logger.Debugf("Creating cluster provider: %s", providerType)

	switch providerType {
	case types.ClusterProviderKind:
		return NewKindProvider(), nil
	case types.ClusterProviderMinikube:
		return NewMinikubeProvider(), nil
	case types.ClusterProviderK3s:
		return nil, utils.NewFrameworkError(utils.ErrCodeInternalError,
			"K3s provider not implemented yet", nil)
	default:
		return nil, utils.NewFrameworkError(utils.ErrCodeInvalidParameter,
			fmt.Sprintf("unsupported cluster provider: %s", providerType), nil)
	}
}

// GetSupportedProviders returns a list of supported cluster providers
func (pf *ProviderFactory) GetSupportedProviders() []types.ClusterProvider {
	return []types.ClusterProvider{
		types.ClusterProviderKind,
		types.ClusterProviderMinikube,
		// types.ClusterProviderK3s,      // Not implemented yet
	}
}

// IsProviderSupported checks if a provider is supported
func (pf *ProviderFactory) IsProviderSupported(providerType types.ClusterProvider) bool {
	supported := pf.GetSupportedProviders()
	for _, supportedType := range supported {
		if providerType == supportedType {
			return true
		}
	}
	return false
}
