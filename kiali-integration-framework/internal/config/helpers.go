package config

import (
	"fmt"
	"strings"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
)

// validateClusterConfig validates cluster configuration
func (cm *ConfigManager) validateClusterConfig() error {
	provider := cm.GetString("cluster.provider")
	if provider == "" {
		return utils.HandleValidationError("cluster.provider", provider, "cluster provider cannot be empty")
	}

	// Validate provider type
	validProviders := []string{
		string(types.ClusterProviderKind),
		string(types.ClusterProviderMinikube),
		string(types.ClusterProviderK3s),
	}

	isValid := false
	for _, validProvider := range validProviders {
		if provider == validProvider {
			isValid = true
			break
		}
	}

	if !isValid {
		return utils.HandleValidationError("cluster.provider", provider,
			fmt.Sprintf("invalid cluster provider, must be one of: %s", strings.Join(validProviders, ", ")))
	}

	name := cm.GetString("cluster.name")
	if name == "" {
		return utils.HandleValidationError("cluster.name", name, "cluster name cannot be empty")
	}

	version := cm.GetString("cluster.version")
	if version == "" {
		return utils.HandleValidationError("cluster.version", version, "cluster version cannot be empty")
	}

	cm.logger.Debugf("Cluster configuration validated: provider=%s, name=%s, version=%s",
		provider, name, version)
	return nil
}

// validateComponentConfigs validates component configurations
func (cm *ConfigManager) validateComponentConfigs() error {
	components := cm.GetStringMap("components")

	for componentName, componentData := range components {
		componentMap, ok := componentData.(map[string]interface{})
		if !ok {
			return utils.HandleValidationError(fmt.Sprintf("components.%s", componentName),
				componentData, "component configuration must be a map")
		}

		// Check if component is enabled
		if enabled, exists := componentMap["enabled"]; exists {
			if enabledVal, ok := enabled.(bool); ok && !enabledVal {
				cm.logger.Debugf("Component %s is disabled, skipping validation", componentName)
				continue
			}
		}

		// Validate version
		if version, exists := componentMap["version"]; exists {
			if versionStr, ok := version.(string); ok && versionStr == "" {
				return utils.HandleValidationError(fmt.Sprintf("components.%s.version", componentName),
					version, "component version cannot be empty")
			}
		}

		cm.logger.Debugf("Component %s configuration validated", componentName)
	}

	return nil
}

// validateTestConfigs validates test configurations
func (cm *ConfigManager) validateTestConfigs() error {
	tests := cm.GetStringMap("tests")

	for testName, testData := range tests {
		testMap, ok := testData.(map[string]interface{})
		if !ok {
			return utils.HandleValidationError(fmt.Sprintf("tests.%s", testName),
				testData, "test configuration must be a map")
		}

		// Check if test is enabled
		if enabled, exists := testMap["enabled"]; exists {
			if enabledVal, ok := enabled.(bool); ok && !enabledVal {
				cm.logger.Debugf("Test %s is disabled, skipping validation", testName)
				continue
			}
		}

		cm.logger.Debugf("Test %s configuration validated", testName)
	}

	return nil
}

// getGlobalConfig builds GlobalConfig from viper
func (cm *ConfigManager) getGlobalConfig() types.GlobalConfig {
	return types.GlobalConfig{
		LogLevel:   cm.GetString("global.logLevel"),
		Timeout:    cm.GetDuration("global.timeout"),
		Verbose:    cm.GetBool("global.verbose"),
		WorkingDir: cm.GetString("global.workingDir"),
		TempDir:    cm.GetString("global.tempDir"),
	}
}

// getClusterConfig builds ClusterConfig from viper
func (cm *ConfigManager) getClusterConfig() types.ClusterConfig {
	return types.ClusterConfig{
		Provider: types.ClusterProvider(cm.GetString("cluster.provider")),
		Name:     cm.GetString("cluster.name"),
		Version:  cm.GetString("cluster.version"),
		Config:   cm.GetStringMap("cluster.config"),
	}
}

// getComponentConfig builds ComponentConfig for a specific component
func (cm *ConfigManager) getComponentConfig(componentName string) *types.ComponentConfig {
	key := fmt.Sprintf("components.%s", componentName)

	if !cm.IsSet(key) {
		return nil
	}

	// Determine component type
	var componentType types.ComponentType
	switch componentName {
	case "istio":
		componentType = types.ComponentTypeIstio
	case "kiali":
		componentType = types.ComponentTypeKiali
	case "prometheus":
		componentType = types.ComponentTypePrometheus
	case "jaeger":
		componentType = types.ComponentTypeJaeger
	case "grafana":
		componentType = types.ComponentTypeGrafana
	default:
		cm.logger.Warnf("Unknown component type: %s", componentName)
		return nil
	}

	config := &types.ComponentConfig{
		Type:    componentType,
		Version: cm.GetString(fmt.Sprintf("%s.version", key)),
		Enabled: cm.GetBool(fmt.Sprintf("%s.enabled", key)),
		Config:  cm.GetStringMap(fmt.Sprintf("%s.config", key)),
	}

	return config
}

// getTestConfig builds TestConfig for a specific test
func (cm *ConfigManager) getTestConfig(testName string) *types.TestConfig {
	key := fmt.Sprintf("tests.%s", testName)

	if !cm.IsSet(key) {
		return nil
	}

	// Determine test type
	var testType types.TestType
	switch testName {
	case "cypress":
		testType = types.TestTypeCypress
	case "go":
		testType = types.TestTypeGo
	default:
		testType = types.TestTypeCustom
	}

	config := &types.TestConfig{
		Type:    testType,
		Enabled: cm.GetBool(fmt.Sprintf("%s.enabled", key)),
		Config:  cm.GetStringMap(fmt.Sprintf("%s.config", key)),
	}

	return config
}

// MergeConfig merges another configuration into this one
func (cm *ConfigManager) MergeConfig(other *ConfigManager) error {
	cm.logger.Info("Merging configuration from another ConfigManager")

	// Get all settings from the other config
	otherSettings := other.viper.AllSettings()

	// Merge each setting
	for key, value := range otherSettings {
		if !cm.IsSet(key) {
			cm.viper.Set(key, value)
			cm.logger.Debugf("Merged configuration key: %s", key)
		}
	}

	cm.logger.Info("Configuration merge completed")
	return nil
}

// GetAllSettings returns all configuration settings as a map
func (cm *ConfigManager) GetAllSettings() map[string]interface{} {
	return cm.viper.AllSettings()
}

// PrintConfig prints the current configuration (for debugging)
func (cm *ConfigManager) PrintConfig() {
	cm.logger.Info("Current configuration:")
	settings := cm.GetAllSettings()

	for key, value := range settings {
		cm.logger.Infof("  %s = %v", key, value)
	}
}
