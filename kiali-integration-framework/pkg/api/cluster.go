package api

import (
	"context"

	"github.com/kiali/kiali-integration-framework/internal/cluster"
	"github.com/kiali/kiali-integration-framework/pkg/types"
)

// ClusterAPI provides a public interface for cluster management
type ClusterAPI interface {
	// Provider management
	CreateProvider(providerType types.ClusterProvider) (types.ClusterProviderInterface, error)
	GetSupportedProviders() []types.ClusterProvider
	IsProviderSupported(providerType types.ClusterProvider) bool

	// Single cluster operations (existing)
	CreateCluster(ctx context.Context, providerType types.ClusterProvider, config types.ClusterConfig) error
	DeleteCluster(ctx context.Context, providerType types.ClusterProvider, name string) error
	GetClusterStatus(ctx context.Context, providerType types.ClusterProvider, name string) (types.ClusterStatus, error)
	GetKubeconfig(ctx context.Context, providerType types.ClusterProvider, name string) (string, error)

	// Multi-cluster operations (new)
	CreateTopology(ctx context.Context, providerType types.ClusterProvider, topology types.ClusterTopology) error
	DeleteTopology(ctx context.Context, providerType types.ClusterProvider, topology types.ClusterTopology) error
	GetTopologyStatus(ctx context.Context, providerType types.ClusterProvider, topology types.ClusterTopology) (types.TopologyStatus, error)
	ListClusters(ctx context.Context, providerType types.ClusterProvider) ([]types.ClusterStatus, error)
}

// clusterAPIImpl implements ClusterAPI
type clusterAPIImpl struct {
	factory *cluster.ProviderFactory
}

// NewClusterAPI creates a new cluster API instance
func NewClusterAPI() ClusterAPI {
	return &clusterAPIImpl{
		factory: cluster.NewProviderFactory(),
	}
}

// CreateProvider creates a cluster provider
func (c *clusterAPIImpl) CreateProvider(providerType types.ClusterProvider) (types.ClusterProviderInterface, error) {
	return c.factory.CreateProvider(providerType)
}

// GetSupportedProviders returns supported cluster providers
func (c *clusterAPIImpl) GetSupportedProviders() []types.ClusterProvider {
	return c.factory.GetSupportedProviders()
}

// IsProviderSupported checks if a provider is supported
func (c *clusterAPIImpl) IsProviderSupported(providerType types.ClusterProvider) bool {
	return c.factory.IsProviderSupported(providerType)
}

// CreateCluster creates a new cluster using the specified provider
func (c *clusterAPIImpl) CreateCluster(ctx context.Context, providerType types.ClusterProvider, config types.ClusterConfig) error {
	provider, err := c.factory.CreateProvider(providerType)
	if err != nil {
		return err
	}

	return provider.Create(ctx, config)
}

// DeleteCluster deletes a cluster using the specified provider
func (c *clusterAPIImpl) DeleteCluster(ctx context.Context, providerType types.ClusterProvider, name string) error {
	provider, err := c.factory.CreateProvider(providerType)
	if err != nil {
		return err
	}

	return provider.Delete(ctx, name)
}

// GetClusterStatus gets the status of a cluster using the specified provider
func (c *clusterAPIImpl) GetClusterStatus(ctx context.Context, providerType types.ClusterProvider, name string) (types.ClusterStatus, error) {
	provider, err := c.factory.CreateProvider(providerType)
	if err != nil {
		return types.ClusterStatus{}, err
	}

	return provider.Status(ctx, name)
}

// GetKubeconfig gets the kubeconfig for a cluster using the specified provider
func (c *clusterAPIImpl) GetKubeconfig(ctx context.Context, providerType types.ClusterProvider, name string) (string, error) {
	provider, err := c.factory.CreateProvider(providerType)
	if err != nil {
		return "", err
	}

	return provider.GetKubeconfig(ctx, name)
}

// CreateTopology creates a multi-cluster topology using the specified provider
func (c *clusterAPIImpl) CreateTopology(ctx context.Context, providerType types.ClusterProvider, topology types.ClusterTopology) error {
	provider, err := c.factory.CreateProvider(providerType)
	if err != nil {
		return err
	}

	return provider.CreateTopology(ctx, topology)
}

// DeleteTopology deletes a multi-cluster topology using the specified provider
func (c *clusterAPIImpl) DeleteTopology(ctx context.Context, providerType types.ClusterProvider, topology types.ClusterTopology) error {
	provider, err := c.factory.CreateProvider(providerType)
	if err != nil {
		return err
	}

	return provider.DeleteTopology(ctx, topology)
}

// GetTopologyStatus gets the status of a multi-cluster topology using the specified provider
func (c *clusterAPIImpl) GetTopologyStatus(ctx context.Context, providerType types.ClusterProvider, topology types.ClusterTopology) (types.TopologyStatus, error) {
	provider, err := c.factory.CreateProvider(providerType)
	if err != nil {
		return types.TopologyStatus{}, err
	}

	return provider.GetTopologyStatus(ctx, topology)
}

// ListClusters lists all clusters managed by the specified provider
func (c *clusterAPIImpl) ListClusters(ctx context.Context, providerType types.ClusterProvider) ([]types.ClusterStatus, error) {
	provider, err := c.factory.CreateProvider(providerType)
	if err != nil {
		return nil, err
	}

	return provider.ListClusters(ctx)
}
