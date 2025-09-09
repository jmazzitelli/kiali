package types

import (
	"context"
	"fmt"
	"time"
)

// ComponentType represents the type of component
type ComponentType string

const (
	ComponentTypeIstio      ComponentType = "istio"
	ComponentTypeKiali      ComponentType = "kiali"
	ComponentTypePrometheus ComponentType = "prometheus"
	ComponentTypeJaeger     ComponentType = "jaeger"
	ComponentTypeGrafana    ComponentType = "grafana"

	// Federation components
	ComponentTypeIstioFederation  ComponentType = "istio-federation"
	ComponentTypeRemoteFederation ComponentType = "remote-federation"
	ComponentTypeGateway          ComponentType = "gateway"

	// Connectivity components
	ComponentTypeNetworkConnectivity ComponentType = "network-connectivity"
)

// ComponentStatus represents the status of a component
type ComponentStatus string

const (
	ComponentStatusNotInstalled ComponentStatus = "not_installed"
	ComponentStatusInstalling   ComponentStatus = "installing"
	ComponentStatusInstalled    ComponentStatus = "installed"
	ComponentStatusFailed       ComponentStatus = "failed"
	ComponentStatusUninstalling ComponentStatus = "uninstalling"
)

// ClusterProvider represents the cluster provider type
type ClusterProvider string

const (
	ClusterProviderKind     ClusterProvider = "kind"
	ClusterProviderMinikube ClusterProvider = "minikube"
	ClusterProviderK3s      ClusterProvider = "k3s"
)

// TestType represents the type of test
type TestType string

const (
	TestTypeCypress TestType = "cypress"
	TestTypeGo      TestType = "go"
	TestTypeCustom  TestType = "custom"
)

// MultiClusterTestType represents types of multi-cluster tests
type MultiClusterTestType string

const (
	MultiClusterTestTypeFederation  MultiClusterTestType = "federation"
	MultiClusterTestTypeTraffic     MultiClusterTestType = "traffic"
	MultiClusterTestTypeDiscovery   MultiClusterTestType = "discovery"
	MultiClusterTestTypeFailover    MultiClusterTestType = "failover"
	MultiClusterTestTypeLoadBalance MultiClusterTestType = "load-balance"
)

// Environment represents the test environment
type Environment struct {
	// Cluster information - single cluster (legacy) or multi-cluster
	Cluster  ClusterConfig   `yaml:"cluster,omitempty" json:"cluster,omitempty"`
	Clusters ClusterTopology `yaml:"clusters,omitempty" json:"clusters,omitempty"`

	// Component configurations
	Components map[string]ComponentConfig `yaml:"components" json:"components"`

	// Test configurations - single cluster tests
	Tests map[string]TestConfig `yaml:"tests" json:"tests"`

	// Multi-cluster test configurations
	MultiClusterTests map[string]MultiClusterTestConfig `yaml:"multiClusterTests,omitempty" json:"multiClusterTests,omitempty"`

	// Global settings
	Global GlobalConfig `yaml:"global" json:"global"`
}

// ClusterConfig represents cluster configuration
type ClusterConfig struct {
	Provider ClusterProvider `yaml:"provider" json:"provider"`
	Name     string          `yaml:"name" json:"name"`
	Version  string          `yaml:"version" json:"version"`
	Config   map[string]any  `yaml:"config" json:"config"`
}

// ComponentConfig represents component configuration
type ComponentConfig struct {
	Type    ComponentType  `yaml:"type" json:"type"`
	Version string         `yaml:"version" json:"version"`
	Enabled bool           `yaml:"enabled" json:"enabled"`
	Config  map[string]any `yaml:"config" json:"config"`
}

// TestConfig represents test configuration
type TestConfig struct {
	Type    TestType       `yaml:"type" json:"type"`
	Enabled bool           `yaml:"enabled" json:"enabled"`
	Config  map[string]any `yaml:"config" json:"config"`
}

// ClusterTopology represents a multi-cluster topology
type ClusterTopology struct {
	// Primary cluster configuration
	Primary ClusterConfig `yaml:"primary" json:"primary"`

	// Remote clusters configuration
	Remotes map[string]ClusterConfig `yaml:"remotes,omitempty" json:"remotes,omitempty"`

	// Federation configuration
	Federation FederationConfig `yaml:"federation,omitempty" json:"federation,omitempty"`

	// Network configuration for cross-cluster communication
	Network NetworkConfig `yaml:"network,omitempty" json:"network,omitempty"`
}

// ClusterRole represents the role of a cluster in a topology
type ClusterRole string

const (
	ClusterRolePrimary ClusterRole = "primary"
	ClusterRoleRemote  ClusterRole = "remote"
)

// FederationConfig represents federation configuration
type FederationConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Service mesh federation settings
	ServiceMesh FederationServiceMesh `yaml:"serviceMesh,omitempty" json:"serviceMesh,omitempty"`

	// Trust domain for cross-cluster communication
	TrustDomain string `yaml:"trustDomain,omitempty" json:"trustDomain,omitempty"`

	// Certificate authority configuration
	CertificateAuthority CertificateAuthorityConfig `yaml:"certificateAuthority,omitempty" json:"certificateAuthority,omitempty"`
}

// FederationServiceMesh represents service mesh federation settings
type FederationServiceMesh struct {
	Type    string         `yaml:"type" json:"type"` // "istio", "linkerd", etc.
	Version string         `yaml:"version" json:"version"`
	Config  map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// CertificateAuthorityConfig represents certificate authority configuration
type CertificateAuthorityConfig struct {
	Type   string         `yaml:"type" json:"type"` // "citadel", "cert-manager", etc.
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// NetworkConfig represents network configuration for cross-cluster communication
type NetworkConfig struct {
	// Gateway configuration for east-west traffic
	Gateway GatewayConfig `yaml:"gateway,omitempty" json:"gateway,omitempty"`

	// Service discovery settings
	ServiceDiscovery ServiceDiscoveryConfig `yaml:"serviceDiscovery,omitempty" json:"serviceDiscovery,omitempty"`

	// Network policies for cross-cluster communication
	Policies []NetworkPolicyConfig `yaml:"policies,omitempty" json:"policies,omitempty"`
}

// GatewayConfig represents gateway configuration
type GatewayConfig struct {
	Type    string         `yaml:"type" json:"type"` // "istio", "nginx", etc.
	Version string         `yaml:"version" json:"version"`
	Config  map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// ServiceDiscoveryConfig represents service discovery configuration
type ServiceDiscoveryConfig struct {
	Type   string         `yaml:"type" json:"type"` // "dns", "api-server", etc.
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// NetworkPolicyConfig represents network policy configuration
type NetworkPolicyConfig struct {
	Name      string         `yaml:"name" json:"name"`
	Namespace string         `yaml:"namespace" json:"namespace"`
	Rules     []PolicyRule   `yaml:"rules" json:"rules"`
	Config    map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// PolicyRule represents a network policy rule
type PolicyRule struct {
	From     []string `yaml:"from,omitempty" json:"from,omitempty"`
	To       []string `yaml:"to,omitempty" json:"to,omitempty"`
	Ports    []string `yaml:"ports,omitempty" json:"ports,omitempty"`
	Protocol string   `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	Action   string   `yaml:"action" json:"action"` // "allow", "deny"
}

// GlobalConfig represents global configuration
type GlobalConfig struct {
	LogLevel   string        `yaml:"logLevel" json:"logLevel"`
	Timeout    time.Duration `yaml:"timeout" json:"timeout"`
	Verbose    bool          `yaml:"verbose" json:"verbose"`
	WorkingDir string        `yaml:"workingDir" json:"workingDir"`
	TempDir    string        `yaml:"tempDir" json:"tempDir"`
}

// Component represents a runtime component instance
type Component struct {
	Name         string          `json:"name"`
	Type         ComponentType   `json:"type"`
	Status       ComponentStatus `json:"status"`
	Version      string          `json:"version"`
	Config       ComponentConfig `json:"config"`
	InstallTime  *time.Time      `json:"installTime,omitempty"`
	LastCheck    *time.Time      `json:"lastCheck,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
}

// TestExecution represents a test execution
type TestExecution struct {
	ID        string        `json:"id"`
	Type      TestType      `json:"type"`
	Status    TestStatus    `json:"status"`
	StartTime time.Time     `json:"startTime"`
	EndTime   *time.Time    `json:"endTime,omitempty"`
	Duration  time.Duration `json:"duration"`
	Config    TestConfig    `json:"config"`
	Results   TestResults   `json:"results"`
	Error     string        `json:"error,omitempty"`
}

// TestStatus represents the status of a test execution
type TestStatus string

const (
	TestStatusPending   TestStatus = "pending"
	TestStatusRunning   TestStatus = "running"
	TestStatusPassed    TestStatus = "passed"
	TestStatusFailed    TestStatus = "failed"
	TestStatusSkipped   TestStatus = "skipped"
	TestStatusCancelled TestStatus = "cancelled"
)

// TestResults represents test execution results
type TestResults struct {
	Total     int               `json:"total"`
	Passed    int               `json:"passed"`
	Failed    int               `json:"failed"`
	Skipped   int               `json:"skipped"`
	Duration  time.Duration     `json:"duration"`
	Artifacts map[string]string `json:"artifacts,omitempty"`
}

// MultiClusterTestResults represents aggregated results from multi-cluster test execution
type MultiClusterTestResults struct {
	OverallResults      TestResults              `json:"overallResults"`
	ClusterResults      map[string]*TestResults  `json:"clusterResults"`
	CrossClusterResults []CrossClusterTestResult `json:"crossClusterResults,omitempty"`
	FederationResults   *FederationTestResults   `json:"federationResults,omitempty"`
	StartTime           time.Time                `json:"startTime"`
	EndTime             time.Time                `json:"endTime"`
	TotalDuration       time.Duration            `json:"totalDuration"`
}

// CrossClusterTestResult represents the result of a cross-cluster test
type CrossClusterTestResult struct {
	TestName         string        `json:"testName"`
	SourceCluster    string        `json:"sourceCluster"`
	TargetClusters   []string      `json:"targetClusters"`
	Status           TestStatus    `json:"status"`
	Duration         time.Duration `json:"duration"`
	Error            string        `json:"error,omitempty"`
	TrafficValidated bool          `json:"trafficValidated"`
	ServiceDiscovery bool          `json:"serviceDiscovery"`
}

// FederationTestResults represents results of federation-specific tests
type FederationTestResults struct {
	TrustDomainValidation   bool                      `json:"trustDomainValidation"`
	CertificateExchange     bool                      `json:"certificateExchange"`
	ServiceMeshConnectivity bool                      `json:"serviceMeshConnectivity"`
	GatewayConfiguration    bool                      `json:"gatewayConfiguration"`
	CrossClusterServices    []FederationServiceResult `json:"crossClusterServices"`
}

// FederationServiceResult represents the result of testing a federated service
type FederationServiceResult struct {
	ServiceName      string        `json:"serviceName"`
	Namespace        string        `json:"namespace"`
	Clusters         []string      `json:"clusters"`
	ConnectivityTest bool          `json:"connectivityTest"`
	LoadBalancing    bool          `json:"loadBalancing"`
	FailoverTest     bool          `json:"failoverTest"`
	Duration         time.Duration `json:"duration"`
	StartTime        time.Time     `json:"startTime"`
	Error            string        `json:"error,omitempty"`
}

// ClusterProviderInterface defines the interface for cluster providers
type ClusterProviderInterface interface {
	Name() ClusterProvider

	// Single cluster operations (existing)
	Create(ctx context.Context, config ClusterConfig) error
	Delete(ctx context.Context, name string) error
	Status(ctx context.Context, name string) (ClusterStatus, error)
	GetKubeconfig(ctx context.Context, name string) (string, error)

	// Multi-cluster operations (new)
	CreateTopology(ctx context.Context, topology ClusterTopology) error
	DeleteTopology(ctx context.Context, topology ClusterTopology) error
	GetTopologyStatus(ctx context.Context, topology ClusterTopology) (TopologyStatus, error)
	ListClusters(ctx context.Context) ([]ClusterStatus, error)
}

// ClusterStatus represents the status of a cluster
type ClusterStatus struct {
	Name      string          `json:"name"`
	Provider  ClusterProvider `json:"provider"`
	State     string          `json:"state"`
	Nodes     int             `json:"nodes"`
	Version   string          `json:"version"`
	CreatedAt time.Time       `json:"createdAt"`
	Healthy   bool            `json:"healthy"`
	Error     string          `json:"error,omitempty"`
}

// TopologyStatus represents the status of a multi-cluster topology
type TopologyStatus struct {
	Primary          ClusterStatus            `json:"primary"`
	Remotes          map[string]ClusterStatus `json:"remotes"`
	OverallHealth    string                   `json:"overallHealth"`              // "healthy", "degraded", "unhealthy"
	FederationStatus string                   `json:"federationStatus,omitempty"` // "enabled", "disabled", "configuring"
	NetworkStatus    string                   `json:"networkStatus,omitempty"`    // "connected", "disconnected", "configuring"
	Error            string                   `json:"error,omitempty"`
}

// ComponentManagerInterface defines the interface for component managers
type ComponentManagerInterface interface {
	Name() string
	Type() ComponentType
	ValidateConfig(config ComponentConfig) error
	Install(ctx context.Context, env *Environment, config ComponentConfig) error
	Uninstall(ctx context.Context, env *Environment, name string) error
	GetStatus(ctx context.Context, env *Environment, component *Component) (ComponentStatus, error)
	Update(ctx context.Context, env *Environment, config ComponentConfig) error
}

// TestExecutorInterface defines the interface for test executors
type TestExecutorInterface interface {
	Name() string
	Type() TestType
	ValidateConfig(config TestConfig) error
	Execute(ctx context.Context, env *Environment, config TestConfig) (*TestResults, error)
	Cancel(ctx context.Context, executionID string) error
	GetStatus(ctx context.Context, executionID string) (TestStatus, error)
}

// MultiClusterTestConfig represents configuration for multi-cluster tests
type MultiClusterTestConfig struct {
	Type            MultiClusterTestType `yaml:"type" json:"type"`
	Enabled         bool                 `yaml:"enabled" json:"enabled"`
	Config          map[string]any       `yaml:"config" json:"config"`
	Clusters        []string             `yaml:"clusters,omitempty" json:"clusters,omitempty"`               // Specific clusters to test
	ExcludeClusters []string             `yaml:"excludeClusters,omitempty" json:"excludeClusters,omitempty"` // Clusters to exclude
	Parallel        bool                 `yaml:"parallel" json:"parallel"`                                   // Run tests in parallel across clusters
	MaxConcurrency  int                  `yaml:"maxConcurrency,omitempty" json:"maxConcurrency,omitempty"`   // Maximum concurrent cluster tests
	Timeout         time.Duration        `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	RetryPolicy     RetryPolicy          `yaml:"retryPolicy,omitempty" json:"retryPolicy,omitempty"`
}

// RetryPolicy defines retry behavior for failed tests
type RetryPolicy struct {
	MaxRetries int           `yaml:"maxRetries" json:"maxRetries"`
	RetryDelay time.Duration `yaml:"retryDelay" json:"retryDelay"`
}

// MultiClusterTestCoordinatorInterface defines the interface for multi-cluster test coordination
type MultiClusterTestCoordinatorInterface interface {
	Name() string
	ValidateConfig(config MultiClusterTestConfig) error
	ExecuteTests(ctx context.Context, env *Environment, config MultiClusterTestConfig) (*MultiClusterTestResults, error)
	CancelExecution(ctx context.Context, executionID string) error
	GetExecutionStatus(ctx context.Context, executionID string) (TestStatus, error)
	ListActiveExecutions(ctx context.Context) ([]string, error)
}

// Helper methods for Environment

// IsMultiCluster returns true if the environment uses multi-cluster configuration
func (e *Environment) IsMultiCluster() bool {
	return e.Clusters.Primary.Name != "" || len(e.Clusters.Remotes) > 0
}

// GetPrimaryCluster returns the primary cluster configuration
func (e *Environment) GetPrimaryCluster() ClusterConfig {
	if e.IsMultiCluster() {
		return e.Clusters.Primary
	}
	return e.Cluster
}

// GetRemoteClusters returns all remote cluster configurations
func (e *Environment) GetRemoteClusters() map[string]ClusterConfig {
	if e.IsMultiCluster() {
		return e.Clusters.Remotes
	}
	return nil
}

// GetAllClusters returns all cluster configurations (primary + remotes)
func (e *Environment) GetAllClusters() map[string]ClusterConfig {
	clusters := make(map[string]ClusterConfig)

	if e.IsMultiCluster() {
		clusters[e.Clusters.Primary.Name] = e.Clusters.Primary
		for name, config := range e.Clusters.Remotes {
			clusters[name] = config
		}
	} else if e.Cluster.Name != "" {
		clusters[e.Cluster.Name] = e.Cluster
	}

	return clusters
}

// GetClusterNames returns a list of all cluster names in the environment
func (e *Environment) GetClusterNames() []string {
	var names []string
	clusters := e.GetAllClusters()
	for name := range clusters {
		names = append(names, name)
	}
	return names
}

// ValidateEnvironment validates the environment configuration
func (e *Environment) ValidateEnvironment() error {
	if e.IsMultiCluster() {
		// Validate multi-cluster configuration
		if e.Clusters.Primary.Name == "" {
			return fmt.Errorf("multi-cluster configuration requires a primary cluster")
		}

		// Check for duplicate cluster names
		clusterNames := make(map[string]bool)
		clusterNames[e.Clusters.Primary.Name] = true

		for name := range e.Clusters.Remotes {
			if clusterNames[name] {
				return fmt.Errorf("duplicate cluster name: %s", name)
			}
			clusterNames[name] = true
		}

		// Validate federation configuration if enabled
		if e.Clusters.Federation.Enabled {
			if e.Clusters.Federation.TrustDomain == "" {
				return fmt.Errorf("federation requires a trust domain")
			}
		}
	} else {
		// Validate single cluster configuration
		if e.Cluster.Name == "" {
			return fmt.Errorf("environment requires cluster configuration")
		}
	}

	// Validate components
	for componentName, componentConfig := range e.Components {
		if componentConfig.Type == "" {
			return fmt.Errorf("component %s missing type", componentName)
		}
		if componentConfig.Version == "" {
			return fmt.Errorf("component %s missing version", componentName)
		}
	}

	// Validate tests
	for testName, testConfig := range e.Tests {
		if testConfig.Type == "" {
			return fmt.Errorf("test %s missing type", testName)
		}
	}

	return nil
}

// GetFederationConfig returns the federation configuration
func (e *Environment) GetFederationConfig() FederationConfig {
	if e.IsMultiCluster() {
		return e.Clusters.Federation
	}
	return FederationConfig{Enabled: false}
}

// GetNetworkConfig returns the network configuration
func (e *Environment) GetNetworkConfig() NetworkConfig {
	if e.IsMultiCluster() {
		return e.Clusters.Network
	}
	return NetworkConfig{}
}
