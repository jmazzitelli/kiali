package config

import (
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/types"
)

func TestConfigManager_LoadDefaults(t *testing.T) {
	cm := NewConfigManager()
	cm.LoadDefaults()

	// Test default values
	if cm.GetString("cluster.provider") != string(types.ClusterProviderKind) {
		t.Errorf("Expected default cluster provider to be %s, got %s",
			types.ClusterProviderKind, cm.GetString("cluster.provider"))
	}

	if cm.GetString("cluster.name") != "kiali-test" {
		t.Errorf("Expected default cluster name to be 'kiali-test', got %s",
			cm.GetString("cluster.name"))
	}

	if !cm.GetBool("components.istio.enabled") {
		t.Error("Expected Istio to be enabled by default")
	}

	if !cm.GetBool("components.kiali.enabled") {
		t.Error("Expected Kiali to be enabled by default")
	}
}

func TestConfigManager_LoadFromString(t *testing.T) {
	cm := NewConfigManager()

	yamlConfig := `
cluster:
  provider: "minikube"
  name: "test-cluster"
  version: "1.28.0"

components:
  istio:
    version: "1.21.0"
    enabled: true

global:
  timeout: "600s"
`

	err := cm.LoadFromString(yamlConfig)
	if err != nil {
		t.Fatalf("Failed to load config from string: %v", err)
	}

	// Test loaded values
	if cm.GetString("cluster.provider") != "minikube" {
		t.Errorf("Expected cluster provider to be 'minikube', got %s",
			cm.GetString("cluster.provider"))
	}

	if cm.GetString("cluster.name") != "test-cluster" {
		t.Errorf("Expected cluster name to be 'test-cluster', got %s",
			cm.GetString("cluster.name"))
	}

	if cm.GetString("components.istio.version") != "1.21.0" {
		t.Errorf("Expected Istio version to be '1.21.0', got %s",
			cm.GetString("components.istio.version"))
	}

	if cm.GetDuration("global.timeout").String() != "10m0s" {
		t.Errorf("Expected timeout to be '10m0s', got %s",
			cm.GetDuration("global.timeout").String())
	}
}

func TestConfigManager_Validate(t *testing.T) {
	cm := NewConfigManager()
	cm.LoadDefaults()

	// Should pass validation with defaults
	err := cm.Validate()
	if err != nil {
		t.Fatalf("Default configuration should pass validation: %v", err)
	}

	// Test invalid cluster provider
	cm.viper.Set("cluster.provider", "invalid-provider")
	err = cm.Validate()
	if err == nil {
		t.Error("Expected validation to fail with invalid cluster provider")
	}

	// Reset to valid provider
	cm.viper.Set("cluster.provider", string(types.ClusterProviderKind))

	// Test empty cluster name
	cm.viper.Set("cluster.name", "")
	err = cm.Validate()
	if err == nil {
		t.Error("Expected validation to fail with empty cluster name")
	}
}

func TestConfigManager_GetEnvironment(t *testing.T) {
	cm := NewConfigManager()
	cm.LoadDefaults()

	env, err := cm.GetEnvironment()
	if err != nil {
		t.Fatalf("Failed to get environment: %v", err)
	}

	if env == nil {
		t.Fatal("Environment should not be nil")
	}

	// Test global config
	if env.Global.LogLevel != "info" {
		t.Errorf("Expected log level to be 'info', got %s", env.Global.LogLevel)
	}

	// Test cluster config
	if env.Cluster.Provider != types.ClusterProviderKind {
		t.Errorf("Expected cluster provider to be %s, got %s",
			types.ClusterProviderKind, env.Cluster.Provider)
	}

	// Test components
	if len(env.Components) == 0 {
		t.Error("Expected components to be populated")
	}

	if istioConfig, exists := env.Components["istio"]; exists {
		if istioConfig.Type != types.ComponentTypeIstio {
			t.Errorf("Expected Istio component type to be %s, got %s",
				types.ComponentTypeIstio, istioConfig.Type)
		}
	} else {
		t.Error("Expected Istio component to exist in environment")
	}
}
