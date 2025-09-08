package main

import (
	"os"
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/api"
)

// TestConfigurationIntegration tests the end-to-end configuration loading
func TestConfigurationIntegration(t *testing.T) {
	// Create a temporary config file
	configContent := `
cluster:
  provider: "kind"
  name: "integration-test"
  version: "1.27.0"

components:
  istio:
    version: "1.20.0"
    enabled: true

global:
  timeout: "300s"
  logLevel: "debug"
`

	tempFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	_, err = tempFile.WriteString(configContent)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tempFile.Close()

	// Test loading configuration
	configAPI := api.NewConfigAPI()
	err = configAPI.LoadFromFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate configuration
	err = configAPI.Validate()
	if err != nil {
		t.Fatalf("Configuration validation failed: %v", err)
	}

	// Test getting environment
	env, err := configAPI.GetEnvironment()
	if err != nil {
		t.Fatalf("Failed to get environment: %v", err)
	}

	// Verify loaded values
	if env.Cluster.Provider != "kind" {
		t.Errorf("Expected cluster provider 'kind', got '%s'", env.Cluster.Provider)
	}

	if env.Cluster.Name != "integration-test" {
		t.Errorf("Expected cluster name 'integration-test', got '%s'", env.Cluster.Name)
	}

	if env.Global.LogLevel != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", env.Global.LogLevel)
	}

	// Verify components
	istioConfig, exists := env.Components["istio"]
	if !exists {
		t.Fatal("Expected Istio component to exist")
	}

	if istioConfig.Version != "1.20.0" {
		t.Errorf("Expected Istio version '1.20.0', got '%s'", istioConfig.Version)
	}

	t.Logf("Configuration integration test passed successfully")
}

// TestEnvironmentVariableOverride tests that environment variables override config file values
func TestEnvironmentVariableOverride(t *testing.T) {
	// Set environment variable
	os.Setenv("KIALI_INT_CLUSTER_NAME", "env-override-test")
	defer os.Unsetenv("KIALI_INT_CLUSTER_NAME")

	// Create config with different value
	configContent := `
cluster:
  provider: "kind"
  name: "config-file-name"
  version: "1.27.0"
`

	tempFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	_, err = tempFile.WriteString(configContent)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tempFile.Close()

	// Load configuration
	configAPI := api.NewConfigAPI()
	err = configAPI.LoadFromFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	// The environment variable should override the config file value
	clusterName := configAPI.GetString("cluster.name")
	if clusterName != "env-override-test" {
		t.Errorf("Expected cluster name to be overridden by env var 'env-override-test', got '%s'", clusterName)
	}

	t.Logf("Environment variable override test passed")
}

// BenchmarkConfigLoading benchmarks configuration loading performance
func BenchmarkConfigLoading(b *testing.B) {
	configContent := `
cluster:
  provider: "kind"
  name: "benchmark-test"
  version: "1.27.0"

components:
  istio:
    version: "1.20.0"
    enabled: true
  kiali:
    version: "latest"
    enabled: true
  prometheus:
    version: "latest"
    enabled: true

tests:
  cypress:
    enabled: true
    baseUrl: "http://localhost:20001"
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		configAPI := api.NewConfigAPI()
		err := configAPI.LoadFromString(configContent)
		if err != nil {
			b.Fatalf("Failed to load config: %v", err)
		}

		_, err = configAPI.GetEnvironment()
		if err != nil {
			b.Fatalf("Failed to get environment: %v", err)
		}
	}
}
