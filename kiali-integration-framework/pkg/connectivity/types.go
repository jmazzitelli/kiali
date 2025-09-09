package connectivity

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

// ConnectivityType represents different types of network connectivity
type ConnectivityType string

const (
	ConnectivityTypeKubernetes ConnectivityType = "kubernetes"
	ConnectivityTypeIstio      ConnectivityType = "istio"
	ConnectivityTypeLinkerd    ConnectivityType = "linkerd"
	ConnectivityTypeManual     ConnectivityType = "manual"
)

// ConnectivityPolicy represents a connectivity policy configuration
type ConnectivityPolicy struct {
	Name        string                 `yaml:"name" json:"name"`
	Type        ConnectivityType       `yaml:"type" json:"type"`
	Version     string                 `yaml:"version" json:"version"`
	Config      map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
	Enabled     bool                   `yaml:"enabled" json:"enabled"`
	Description string                 `yaml:"description,omitempty" json:"description,omitempty"`
}

// ConnectivityStatus represents the status of connectivity
type ConnectivityStatus struct {
	Type           ConnectivityType `json:"type"`
	State          string           `json:"state"`
	Healthy        bool             `json:"healthy"`
	LastChecked    string           `json:"lastChecked,omitempty"`
	ErrorMessage   string           `json:"errorMessage,omitempty"`
	PoliciesCount  int              `json:"policiesCount"`
	ServicesCount  int              `json:"servicesCount"`
	EndpointsCount int              `json:"endpointsCount"`
}

// ConnectivityTemplate represents a connectivity configuration template
type ConnectivityTemplate struct {
	Name        string                 `yaml:"name" json:"name"`
	Type        ConnectivityType       `yaml:"type" json:"type"`
	Description string                 `yaml:"description" json:"description"`
	Version     string                 `yaml:"version" json:"version"`
	Policies    []ConnectivityPolicy   `yaml:"policies" json:"policies"`
	Config      map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
	Tags        []string               `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// ConnectivityHealthCheck represents a connectivity health check
type ConnectivityHealthCheck struct {
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Healthy  bool                   `json:"healthy"`
	Message  string                 `json:"message,omitempty"`
	LastRun  string                 `json:"lastRun"`
	Duration string                 `json:"duration"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// ConnectivityProvider interface defines the contract for connectivity implementations
type ConnectivityProvider interface {
	Type() ConnectivityType
	Install(ctx context.Context, clientset *kubernetes.Clientset, config map[string]interface{}) error
	Uninstall(ctx context.Context, clientset *kubernetes.Clientset) error
	Status(ctx context.Context, clientset *kubernetes.Clientset) (ConnectivityStatus, error)
	HealthCheck(ctx context.Context, clientset *kubernetes.Clientset) ([]ConnectivityHealthCheck, error)
	ValidateConfig(config map[string]interface{}) error
}

// TrafficPolicy represents a traffic routing policy
type TrafficPolicy struct {
	Name        string                 `yaml:"name" json:"name"`
	Type        string                 `yaml:"type" json:"type"`
	Source      TrafficPolicyEndpoint  `yaml:"source" json:"source"`
	Destination TrafficPolicyEndpoint  `yaml:"destination" json:"destination"`
	Rules       []TrafficRule          `yaml:"rules" json:"rules"`
	Config      map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// TrafficPolicyEndpoint represents an endpoint in a traffic policy
type TrafficPolicyEndpoint struct {
	Service   string            `yaml:"service" json:"service"`
	Namespace string            `yaml:"namespace" json:"namespace"`
	Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// TrafficRule represents a traffic routing rule
type TrafficRule struct {
	Name      string                 `yaml:"name" json:"name"`
	Condition string                 `yaml:"condition,omitempty" json:"condition,omitempty"`
	Action    string                 `yaml:"action" json:"action"`
	Weight    int                    `yaml:"weight,omitempty" json:"weight,omitempty"`
	Config    map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// ServiceDiscoveryConfig represents service discovery configuration
type ServiceDiscoveryConfig struct {
	Enabled  bool                   `yaml:"enabled" json:"enabled"`
	Domains  []string               `yaml:"domains,omitempty" json:"domains,omitempty"`
	Clusters []string               `yaml:"clusters,omitempty" json:"clusters,omitempty"`
	Config   map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// DNSConfig represents DNS configuration for cross-cluster communication
type DNSConfig struct {
	Enabled       bool                   `yaml:"enabled" json:"enabled"`
	Clusters      []string               `yaml:"clusters" json:"clusters"`
	SearchDomains []string               `yaml:"searchDomains,omitempty" json:"searchDomains,omitempty"`
	Nameservers   []string               `yaml:"nameservers,omitempty" json:"nameservers,omitempty"`
	Config        map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// LoadBalancingConfig represents load balancing configuration
type LoadBalancingConfig struct {
	Enabled bool                   `yaml:"enabled" json:"enabled"`
	Type    string                 `yaml:"type" json:"type"`
	Config  map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// HealthCheckConfig represents health check configuration
type HealthCheckConfig struct {
	Enabled          bool                   `yaml:"enabled" json:"enabled"`
	Interval         string                 `yaml:"interval" json:"interval"`
	Timeout          string                 `yaml:"timeout" json:"timeout"`
	FailureThreshold int                    `yaml:"failureThreshold" json:"failureThreshold"`
	Config           map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}
