package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// ConfigManager manages configuration loading and validation
type ConfigManager struct {
	viper      *viper.Viper
	loaded     bool
	configPath string
	logger     *utils.Logger
}

// NewConfigManager creates a new configuration manager
func NewConfigManager() *ConfigManager {
	v := viper.New()
	v.SetEnvPrefix("KIALI_INT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	return &ConfigManager{
		viper:  v,
		logger: utils.GetGlobalLogger(),
	}
}

// LoadFromFile loads configuration from a YAML file
func (cm *ConfigManager) LoadFromFile(configPath string) error {
	cm.logger.Infof("Loading configuration from file: %s", configPath)

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return utils.NewFrameworkError(utils.ErrCodeConfigNotFound, "configuration file not found", err)
	}

	cm.viper.SetConfigFile(configPath)

	if err := cm.viper.ReadInConfig(); err != nil {
		return utils.WrapError(err, utils.ErrCodeConfigParseFailed, "failed to parse configuration file")
	}

	cm.configPath = configPath
	cm.loaded = true

	cm.logger.Infof("Configuration loaded successfully from: %s", configPath)
	return nil
}

// LoadFromReader loads configuration from an io.Reader
func (cm *ConfigManager) LoadFromReader(reader io.Reader) error {
	cm.logger.Info("Loading configuration from reader")

	cm.viper.SetConfigType("yaml")
	if err := cm.viper.ReadConfig(reader); err != nil {
		return utils.WrapError(err, utils.ErrCodeConfigParseFailed, "failed to parse configuration from reader")
	}

	cm.loaded = true
	cm.logger.Info("Configuration loaded successfully from reader")
	return nil
}

// LoadFromString loads configuration from a YAML string
func (cm *ConfigManager) LoadFromString(yamlContent string) error {
	cm.logger.Info("Loading configuration from string")

	cm.viper.SetConfigType("yaml")
	if err := cm.viper.ReadConfig(strings.NewReader(yamlContent)); err != nil {
		return utils.WrapError(err, utils.ErrCodeConfigParseFailed, "failed to parse configuration string")
	}

	cm.loaded = true
	cm.logger.Info("Configuration loaded successfully from string")
	return nil
}

// LoadDefaults sets default configuration values
func (cm *ConfigManager) LoadDefaults() {
	cm.logger.Debug("Setting default configuration values")

	// Global settings
	cm.viper.SetDefault("global.timeout", "300s")
	cm.viper.SetDefault("global.workingDir", ".")
	cm.viper.SetDefault("global.tempDir", filepath.Join(os.TempDir(), "kiali-integration-framework"))
	cm.viper.SetDefault("global.logLevel", "info")
	cm.viper.SetDefault("global.verbose", false)

	// Cluster settings
	cm.viper.SetDefault("cluster.provider", string(types.ClusterProviderKind))
	cm.viper.SetDefault("cluster.name", "kiali-test")
	cm.viper.SetDefault("cluster.version", "1.27.0")
	cm.viper.SetDefault("cluster.config", map[string]interface{}{})

	// Component settings
	cm.viper.SetDefault("components.istio.version", "1.20.0")
	cm.viper.SetDefault("components.istio.enabled", true)
	cm.viper.SetDefault("components.istio.profile", "default")
	cm.viper.SetDefault("components.istio.config", map[string]interface{}{})

	cm.viper.SetDefault("components.kiali.version", "latest")
	cm.viper.SetDefault("components.kiali.enabled", true)
	cm.viper.SetDefault("components.kiali.auth.strategy", "token")
	cm.viper.SetDefault("components.kiali.config", map[string]interface{}{})

	cm.viper.SetDefault("components.prometheus.version", "latest")
	cm.viper.SetDefault("components.prometheus.enabled", true)
	cm.viper.SetDefault("components.prometheus.config", map[string]interface{}{})

	// Test settings
	cm.viper.SetDefault("tests.cypress.baseUrl", "http://localhost:20001")
	cm.viper.SetDefault("tests.cypress.timeout", 60000)
	cm.viper.SetDefault("tests.cypress.enabled", true)
	cm.viper.SetDefault("tests.cypress.config", map[string]interface{}{})
}

// Validate validates the loaded configuration
func (cm *ConfigManager) Validate() error {
	cm.logger.Info("Validating configuration")

	// Validate cluster configuration
	if err := cm.validateClusterConfig(); err != nil {
		return err
	}

	// Validate component configurations
	if err := cm.validateComponentConfigs(); err != nil {
		return err
	}

	// Validate test configurations
	if err := cm.validateTestConfigs(); err != nil {
		return err
	}

	cm.logger.Info("Configuration validation completed successfully")
	return nil
}

// GetEnvironment builds an Environment struct from the configuration
func (cm *ConfigManager) GetEnvironment() (*types.Environment, error) {
	cm.logger.Info("Building environment configuration")

	env := &types.Environment{
		Global:     cm.getGlobalConfig(),
		Cluster:    cm.getClusterConfig(),
		Components: make(map[string]types.ComponentConfig),
		Tests:      make(map[string]types.TestConfig),
	}

	// Load component configurations
	for componentName := range cm.viper.GetStringMap("components") {
		if config := cm.getComponentConfig(componentName); config != nil {
			env.Components[componentName] = *config
		}
	}

	// Load test configurations
	for testName := range cm.viper.GetStringMap("tests") {
		if config := cm.getTestConfig(testName); config != nil {
			env.Tests[testName] = *config
		}
	}

	return env, nil
}

// Get returns a configuration value
func (cm *ConfigManager) Get(key string) interface{} {
	return cm.viper.Get(key)
}

// GetString returns a string configuration value
func (cm *ConfigManager) GetString(key string) string {
	return cm.viper.GetString(key)
}

// GetBool returns a boolean configuration value
func (cm *ConfigManager) GetBool(key string) bool {
	return cm.viper.GetBool(key)
}

// GetInt returns an integer configuration value
func (cm *ConfigManager) GetInt(key string) int {
	return cm.viper.GetInt(key)
}

// GetDuration returns a duration configuration value
func (cm *ConfigManager) GetDuration(key string) time.Duration {
	return cm.viper.GetDuration(key)
}

// GetStringSlice returns a string slice configuration value
func (cm *ConfigManager) GetStringSlice(key string) []string {
	return cm.viper.GetStringSlice(key)
}

// GetStringMap returns a string map configuration value
func (cm *ConfigManager) GetStringMap(key string) map[string]interface{} {
	return cm.viper.GetStringMap(key)
}

// IsSet checks if a configuration key is set
func (cm *ConfigManager) IsSet(key string) bool {
	return cm.viper.IsSet(key)
}

// SaveToFile saves the current configuration to a file
func (cm *ConfigManager) SaveToFile(configPath string) error {
	cm.logger.Infof("Saving configuration to file: %s", configPath)

	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return utils.WrapError(err, utils.ErrCodeFileSystemError, "failed to create config directory")
	}

	// Get all settings
	settings := cm.viper.AllSettings()

	// Convert to YAML
	yamlData, err := yaml.Marshal(settings)
	if err != nil {
		return utils.WrapError(err, utils.ErrCodeConfigParseFailed, "failed to marshal configuration to YAML")
	}

	// Write to file
	if err := os.WriteFile(configPath, yamlData, 0644); err != nil {
		return utils.WrapError(err, utils.ErrCodeFileSystemError, "failed to write configuration file")
	}

	cm.logger.Infof("Configuration saved successfully to: %s", configPath)
	return nil
}

// GetConfigPath returns the current configuration file path
func (cm *ConfigManager) GetConfigPath() string {
	return cm.configPath
}

// IsLoaded returns whether configuration has been loaded
func (cm *ConfigManager) IsLoaded() bool {
	return cm.loaded
}
