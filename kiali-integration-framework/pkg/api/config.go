package api

import (
	"io"
	"time"

	"github.com/kiali/kiali-integration-framework/internal/config"
	"github.com/kiali/kiali-integration-framework/pkg/types"
)

// ConfigAPI provides a public interface for configuration management
type ConfigAPI interface {
	// Loading methods
	LoadFromFile(configPath string) error
	LoadFromReader(reader io.Reader) error
	LoadFromString(yamlContent string) error
	LoadDefaults()

	// Validation
	Validate() error

	// Getters
	GetEnvironment() (*types.Environment, error)
	Get(key string) interface{}
	GetString(key string) string
	GetBool(key string) bool
	GetInt(key string) int
	GetDuration(key string) time.Duration
	GetStringSlice(key string) []string
	GetStringMap(key string) map[string]interface{}
	IsSet(key string) bool

	// Persistence
	SaveToFile(configPath string) error

	// Status
	GetConfigPath() string
	IsLoaded() bool

	// Utility
	PrintConfig()
}

// NewConfigAPI creates a new configuration API instance
func NewConfigAPI() ConfigAPI {
	return config.NewConfigManager()
}
