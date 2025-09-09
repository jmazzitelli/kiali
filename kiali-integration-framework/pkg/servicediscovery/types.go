package servicediscovery

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
)

// ServiceDiscoveryType represents different types of service discovery
type ServiceDiscoveryType string

const (
	ServiceDiscoveryTypeDNS         ServiceDiscoveryType = "dns"
	ServiceDiscoveryTypeAPIServer   ServiceDiscoveryType = "api-server"
	ServiceDiscoveryTypePropagation ServiceDiscoveryType = "propagation"
	ServiceDiscoveryTypeManual      ServiceDiscoveryType = "manual"
)

// ServiceDiscoveryConfig represents service discovery configuration
type ServiceDiscoveryConfig struct {
	Enabled  bool                   `yaml:"enabled" json:"enabled"`
	Type     ServiceDiscoveryType   `yaml:"type" json:"type"`
	Domains  []string               `yaml:"domains,omitempty" json:"domains,omitempty"`
	Clusters []string               `yaml:"clusters,omitempty" json:"clusters,omitempty"`
	Config   map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// ServiceDiscoveryStatus represents the status of service discovery
type ServiceDiscoveryStatus struct {
	Type           ServiceDiscoveryType `json:"type"`
	State          string               `json:"state"`
	Healthy        bool                 `json:"healthy"`
	ServicesCount  int                  `json:"servicesCount"`
	EndpointsCount int                  `json:"endpointsCount"`
	LastChecked    time.Time            `json:"lastChecked"`
	ErrorMessage   string               `json:"errorMessage,omitempty"`
	Clusters       []string             `json:"clusters,omitempty"`
}

// ServiceDiscoveryProvider interface defines the contract for service discovery implementations
type ServiceDiscoveryProvider interface {
	Type() ServiceDiscoveryType
	Install(ctx context.Context, clientset kubernetes.Interface, config map[string]interface{}) error
	Uninstall(ctx context.Context, clientset kubernetes.Interface) error
	Status(ctx context.Context, clientset kubernetes.Interface) (ServiceDiscoveryStatus, error)
	HealthCheck(ctx context.Context, clientset kubernetes.Interface) ([]ServiceDiscoveryHealthCheck, error)
	ValidateConfig(config map[string]interface{}) error
}

// ServiceDiscoveryHealthCheck represents a service discovery health check
type ServiceDiscoveryHealthCheck struct {
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Healthy  bool                   `json:"healthy"`
	Message  string                 `json:"message,omitempty"`
	LastRun  time.Time              `json:"lastRun"`
	Duration time.Duration          `json:"duration"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// DNSConfig represents DNS configuration for service discovery
type DNSConfig struct {
	Enabled       bool                   `yaml:"enabled" json:"enabled"`
	Clusters      []string               `yaml:"clusters" json:"clusters"`
	SearchDomains []string               `yaml:"searchDomains,omitempty" json:"searchDomains,omitempty"`
	Nameservers   []string               `yaml:"nameservers,omitempty" json:"nameservers,omitempty"`
	TTL           int                    `yaml:"ttl,omitempty" json:"ttl,omitempty"`
	Config        map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// APIServerConfig represents API server aggregation configuration
type APIServerConfig struct {
	Enabled      bool                   `yaml:"enabled" json:"enabled"`
	Clusters     []string               `yaml:"clusters" json:"clusters"`
	APIServerURL string                 `yaml:"apiServerUrl,omitempty" json:"apiServerUrl,omitempty"`
	CACert       string                 `yaml:"caCert,omitempty" json:"caCert,omitempty"`
	ClientCert   string                 `yaml:"clientCert,omitempty" json:"clientCert,omitempty"`
	ClientKey    string                 `yaml:"clientKey,omitempty" json:"clientKey,omitempty"`
	Config       map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// ServicePropagationConfig represents service propagation configuration
type ServicePropagationConfig struct {
	Enabled        bool                   `yaml:"enabled" json:"enabled"`
	Clusters       []string               `yaml:"clusters" json:"clusters"`
	SelectorLabels map[string]string      `yaml:"selectorLabels,omitempty" json:"selectorLabels,omitempty"`
	ExcludeLabels  map[string]string      `yaml:"excludeLabels,omitempty" json:"excludeLabels,omitempty"`
	Namespaces     []string               `yaml:"namespaces,omitempty" json:"namespaces,omitempty"`
	SyncInterval   time.Duration          `yaml:"syncInterval,omitempty" json:"syncInterval,omitempty"`
	Config         map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// ServiceEndpoint represents a service endpoint for propagation
type ServiceEndpoint struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Cluster     string            `json:"cluster"`
	Type        string            `json:"type"`
	Ports       []ServicePort     `json:"ports"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Address     string            `json:"address,omitempty"`
	LastUpdated time.Time         `json:"lastUpdated"`
}

// ServicePort represents a service port
type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort int32  `json:"targetPort,omitempty"`
	Protocol   string `json:"protocol"`
}

// ServicePropagationStatus represents the status of service propagation
type ServicePropagationStatus struct {
	SourceCluster       string                     `json:"sourceCluster"`
	TargetClusters      []string                   `json:"targetClusters"`
	ServicesPropagated  int                        `json:"servicesPropagated"`
	EndpointsPropagated int                        `json:"endpointsPropagated"`
	LastSyncTime        time.Time                  `json:"lastSyncTime"`
	SyncStatus          string                     `json:"syncStatus"`
	ErrorMessage        string                     `json:"errorMessage,omitempty"`
	ServiceStatuses     []ServicePropagationResult `json:"serviceStatuses,omitempty"`
}

// ServicePropagationResult represents the result of propagating a service
type ServicePropagationResult struct {
	ServiceName   string    `json:"serviceName"`
	Namespace     string    `json:"namespace"`
	Cluster       string    `json:"cluster"`
	Propagated    bool      `json:"propagated"`
	ErrorMessage  string    `json:"errorMessage,omitempty"`
	LastAttempted time.Time `json:"lastAttempted"`
}
