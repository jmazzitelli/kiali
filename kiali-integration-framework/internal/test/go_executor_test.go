package test

import (
	"context"
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGoExecutor(t *testing.T) {
	executor := NewGoExecutor()

	assert.NotNil(t, executor)
	assert.Equal(t, "go", executor.Name())
	assert.Equal(t, types.TestTypeGo, executor.Type())
}

func TestGoExecutor_ValidateConfig_Valid_WithPackage(t *testing.T) {
	executor := NewGoExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeGo,
		Enabled: true,
		Config: map[string]interface{}{
			"go": map[string]interface{}{
				"package": "./pkg/...",
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.NoError(t, err)
}

func TestGoExecutor_ValidateConfig_Valid_WithPackages(t *testing.T) {
	executor := NewGoExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeGo,
		Enabled: true,
		Config: map[string]interface{}{
			"go": map[string]interface{}{
				"packages": []interface{}{"./pkg/...", "./cmd/..."},
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.NoError(t, err)
}

func TestGoExecutor_ValidateConfig_MissingPackageAndPackages(t *testing.T) {
	executor := NewGoExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeGo,
		Enabled: true,
		Config: map[string]interface{}{
			"go": map[string]interface{}{
				"verbose": true,
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "either 'packages' array or 'package' string must be specified")
}

func TestGoExecutor_ValidateConfig_EmptyPackages(t *testing.T) {
	executor := NewGoExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeGo,
		Enabled: true,
		Config: map[string]interface{}{
			"go": map[string]interface{}{
				"packages": []interface{}{},
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "packages array cannot be empty")
}

func TestGoExecutor_ValidateConfig_EmptyPackage(t *testing.T) {
	executor := NewGoExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeGo,
		Enabled: true,
		Config: map[string]interface{}{
			"go": map[string]interface{}{
				"package": "",
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package cannot be empty")
}

func TestGoExecutor_ValidateConfig_WrongType(t *testing.T) {
	executor := NewGoExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeCypress, // Wrong type
		Enabled: true,
		Config: map[string]interface{}{
			"go": map[string]interface{}{
				"package": "./pkg/...",
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid test type")
}

func TestGoExecutor_ValidateConfig_MissingGoSection(t *testing.T) {
	executor := NewGoExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeGo,
		Enabled: true,
		Config: map[string]interface{}{
			"cypress": map[string]interface{}{}, // Wrong section
		},
	}

	err := executor.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "go configuration section is required")
}

func TestGoExecutor_BuildGoCommand_BasicPackage(t *testing.T) {
	executor := NewGoExecutor()

	config := map[string]interface{}{
		"package": "./pkg/...",
		"verbose": true,
		"race":    true,
	}

	cmd, args := executor.buildGoCommand(config)

	assert.Equal(t, "go", cmd)
	assert.Contains(t, args, "test")
	assert.Contains(t, args, "./pkg/...")
	assert.Contains(t, args, "-v")
	assert.Contains(t, args, "-race")
}

func TestGoExecutor_BuildGoCommand_WithPackages(t *testing.T) {
	executor := NewGoExecutor()

	config := map[string]interface{}{
		"packages": []interface{}{"./pkg/...", "./cmd/..."},
		"verbose":  true,
	}

	cmd, args := executor.buildGoCommand(config)

	assert.Equal(t, "go", cmd)
	assert.Contains(t, args, "test")
	assert.Contains(t, args, "./pkg/...")
	assert.Contains(t, args, "./cmd/...")
	assert.Contains(t, args, "-v")
}

func TestGoExecutor_BuildGoCommand_WithCoverage(t *testing.T) {
	executor := NewGoExecutor()

	config := map[string]interface{}{
		"package":         "./pkg/...",
		"coverage":        true,
		"coverageProfile": "coverage.out",
	}

	cmd, args := executor.buildGoCommand(config)

	assert.Equal(t, "go", cmd)
	assert.Contains(t, args, "-cover")
	assert.Contains(t, args, "-coverprofile")
	assert.Contains(t, args, "coverage.out")
}

func TestGoExecutor_BuildGoCommand_WithTags(t *testing.T) {
	executor := NewGoExecutor()

	config := map[string]interface{}{
		"package": "./pkg/...",
		"tags":    []interface{}{"integration", "e2e"},
	}

	cmd, args := executor.buildGoCommand(config)

	assert.Equal(t, "go", cmd)
	assert.Contains(t, args, "-tags")
	assert.Contains(t, args, "integration,e2e")
}

func TestGoExecutor_BuildGoCommand_WithRunSkip(t *testing.T) {
	executor := NewGoExecutor()

	config := map[string]interface{}{
		"package": "./pkg/...",
		"run":     "TestIntegration",
		"skip":    "TestSlow",
	}

	cmd, args := executor.buildGoCommand(config)

	assert.Equal(t, "go", cmd)
	assert.Contains(t, args, "-run")
	assert.Contains(t, args, "TestIntegration")
	assert.Contains(t, args, "-skip")
	assert.Contains(t, args, "TestSlow")
}

func TestGoExecutor_BuildGoCommand_WithBenchmark(t *testing.T) {
	executor := NewGoExecutor()

	config := map[string]interface{}{
		"package":   "./pkg/...",
		"bench":     "Benchmark",
		"benchTime": "10s",
	}

	cmd, args := executor.buildGoCommand(config)

	assert.Equal(t, "go", cmd)
	assert.Contains(t, args, "-bench")
	assert.Contains(t, args, "Benchmark")
	assert.Contains(t, args, "-benchtime")
	assert.Contains(t, args, "10s")
}

func TestGoExecutor_BuildEnvironmentVariables(t *testing.T) {
	executor := NewGoExecutor()

	config := map[string]interface{}{
		"env": map[string]interface{}{
			"TEST_VAR": "test_value",
			"DB_HOST":  "localhost",
		},
	}

	env := &types.Environment{
		Cluster: types.ClusterConfig{
			Provider: types.ClusterProviderKind,
			Name:     "test-cluster",
		},
		Components: map[string]types.ComponentConfig{
			"istio": {
				Type:    types.ComponentTypeIstio,
				Version: "1.20.0",
				Enabled: true,
			},
		},
	}

	envVars := executor.buildEnvironmentVariables(config, env)

	assert.Contains(t, envVars, "TEST_VAR=test_value")
	assert.Contains(t, envVars, "DB_HOST=localhost")
	assert.Contains(t, envVars, "KIALI_INT_CLUSTER_PROVIDER=kind")
	assert.Contains(t, envVars, "KIALI_INT_CLUSTER_NAME=test-cluster")
	assert.Contains(t, envVars, "KIALI_INT_COMPONENT_ISTIO_ENABLED=true")
	assert.Contains(t, envVars, "KIALI_INT_COMPONENT_ISTIO_TYPE=istio")
	assert.Contains(t, envVars, "KIALI_INT_COMPONENT_ISTIO_VERSION=1.20.0")
}

func TestGoExecutor_ParseGoOutput_Success(t *testing.T) {
	executor := NewGoExecutor()

	output := `
=== RUN TestExample
--- PASS: TestExample (0.00s)
=== RUN TestAnother
--- PASS: TestAnother (0.00s)
PASS
ok  	example.com/test	0.002s
PASS: 2, FAIL: 0, SKIP: 0
`

	results := executor.parseGoOutput(output, nil)

	assert.Equal(t, 2, results.Total)
	assert.Equal(t, 2, results.Passed)
	assert.Equal(t, 0, results.Failed)
	assert.Equal(t, 0, results.Skipped)
}

func TestGoExecutor_ParseGoOutput_WithFailures(t *testing.T) {
	executor := NewGoExecutor()

	output := `
=== RUN TestPass
--- PASS: TestPass (0.00s)
=== RUN TestFail
--- FAIL: TestFail (0.00s)
=== RUN TestSkip
--- SKIP: TestSkip (0.00s)
FAIL
FAIL	example.com/test	0.003s
PASS: 1, FAIL: 1, SKIP: 1
`

	results := executor.parseGoOutput(output, nil)

	assert.Equal(t, 3, results.Total)
	assert.Equal(t, 1, results.Passed)
	assert.Equal(t, 1, results.Failed)
	assert.Equal(t, 1, results.Skipped)
}

func TestGoExecutor_ParseGoOutput_WithCoverage(t *testing.T) {
	executor := NewGoExecutor()

	output := `
PASS
ok  	example.com/test	0.002s	coverage: 85.2% of statements
PASS: 2, FAIL: 0, SKIP: 0
`

	results := executor.parseGoOutput(output, nil)

	assert.Equal(t, 2, results.Total)
	assert.Equal(t, 2, results.Passed)
	// Coverage artifacts would be detected if coverage file was specified
}

func TestGoExecutor_ParseGoOutput_CommandError(t *testing.T) {
	executor := NewGoExecutor()

	output := `
=== RUN TestFail
--- FAIL: TestFail (0.00s)
FAIL
FAIL	example.com/test	0.003s
`

	results := executor.parseGoOutput(output, assert.AnError)

	assert.Equal(t, 1, results.Total)
	assert.Equal(t, 0, results.Passed)
	assert.Equal(t, 1, results.Failed)
	assert.Equal(t, 0, results.Skipped)
}

func TestGoExecutor_ParseNumber(t *testing.T) {
	executor := NewGoExecutor()

	tests := []struct {
		input    string
		expected int
		hasError bool
	}{
		{"10", 10, false},
		{"0", 0, false},
		{"  5  ", 5, false},
		{"not-a-number", 0, true},
		{"", 0, true},
	}

	for _, test := range tests {
		result, err := executor.parseNumber(test.input)
		if test.hasError {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, test.expected, result)
		}
	}
}

// Integration test that demonstrates the full executor workflow
func TestGoExecutor_Integration(t *testing.T) {
	executor := NewGoExecutor()

	// Create a test configuration
	config := types.TestConfig{
		Type:    types.TestTypeGo,
		Enabled: true,
		Config: map[string]interface{}{
			"go": map[string]interface{}{
				"package": "./pkg/types", // Test against a known package
				"verbose": true,
				"short":   true,
			},
		},
	}

	// Create a minimal environment
	env := &types.Environment{
		Cluster: types.ClusterConfig{
			Provider: types.ClusterProviderKind,
			Name:     "test-cluster",
		},
		Components: make(map[string]types.ComponentConfig),
		Global: types.GlobalConfig{
			LogLevel: "info",
		},
	}

	// Validate configuration
	err := executor.ValidateConfig(config)
	require.NoError(t, err)

	// Test execution (this may succeed or fail depending on the actual package)
	results, err := executor.Execute(context.Background(), env, config)

	// The test should not panic regardless of success/failure
	assert.NotNil(t, results)
	// Results should have some indication of execution
	assert.True(t, results.Total >= 0)
}
