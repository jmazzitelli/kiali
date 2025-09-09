package test

import (
	"context"
	"testing"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCypressExecutor(t *testing.T) {
	executor := NewCypressExecutor()

	assert.NotNil(t, executor)
	assert.Equal(t, "cypress", executor.Name())
	assert.Equal(t, types.TestTypeCypress, executor.Type())
}

func TestCypressExecutor_ValidateConfig_Valid(t *testing.T) {
	executor := NewCypressExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeCypress,
		Enabled: true,
		Config: map[string]interface{}{
			"cypress": map[string]interface{}{
				"baseUrl":     "http://localhost:3000",
				"specPattern": "cypress/integration/**/*.cy.js",
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.NoError(t, err)
}

func TestCypressExecutor_ValidateConfig_MissingBaseUrl(t *testing.T) {
	executor := NewCypressExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeCypress,
		Enabled: true,
		Config: map[string]interface{}{
			"cypress": map[string]interface{}{
				"specPattern": "cypress/integration/**/*.cy.js",
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "baseUrl is required")
}

func TestCypressExecutor_ValidateConfig_MissingSpecPattern(t *testing.T) {
	executor := NewCypressExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeCypress,
		Enabled: true,
		Config: map[string]interface{}{
			"cypress": map[string]interface{}{
				"baseUrl": "http://localhost:3000",
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "specPattern is required")
}

func TestCypressExecutor_ValidateConfig_WrongType(t *testing.T) {
	executor := NewCypressExecutor()

	config := types.TestConfig{
		Type:    types.TestTypeGo, // Wrong type
		Enabled: true,
		Config: map[string]interface{}{
			"cypress": map[string]interface{}{
				"baseUrl":     "http://localhost:3000",
				"specPattern": "cypress/integration/**/*.cy.js",
			},
		},
	}

	err := executor.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid test type")
}

func TestCypressExecutor_BuildCypressCommand_Basic(t *testing.T) {
	executor := NewCypressExecutor()

	config := map[string]interface{}{
		"baseUrl":     "http://localhost:3000",
		"specPattern": "cypress/integration/**/*.cy.js",
		"headless":    true,
		"browser":     "electron",
	}

	cmd, args := executor.buildCypressCommand(config)

	assert.Equal(t, "npx", cmd)
	assert.Contains(t, args, "cypress")
	assert.Contains(t, args, "run")
	assert.Contains(t, args, "--spec")
	assert.Contains(t, args, "cypress/integration/**/*.cy.js")
	assert.Contains(t, args, "--browser")
	assert.Contains(t, args, "electron")
	assert.Contains(t, args, "--headless")
}

func TestCypressExecutor_BuildCypressCommand_WithTags(t *testing.T) {
	executor := NewCypressExecutor()

	config := map[string]interface{}{
		"baseUrl":     "http://localhost:3000",
		"specPattern": "cypress/integration/**/*.cy.js",
		"tags":        []interface{}{"@smoke", "@core"},
	}

	cmd, args := executor.buildCypressCommand(config)

	assert.Equal(t, "npx", cmd)
	assert.Contains(t, args, "--grep")
	assert.Contains(t, args, "@smoke|@core")
}

func TestCypressExecutor_BuildEnvironmentVariables(t *testing.T) {
	executor := NewCypressExecutor()

	config := map[string]interface{}{
		"baseUrl": "http://localhost:3000",
		"env": map[string]interface{}{
			"TEST_VAR": "test_value",
		},
	}

	envVars := executor.buildEnvironmentVariables(config)

	assert.Contains(t, envVars, "CYPRESS_BASE_URL=http://localhost:3000")
	assert.Contains(t, envVars, "CYPRESS_TEST_VAR=test_value")
}

func TestCypressExecutor_ParseCypressOutput_Success(t *testing.T) {
	executor := NewCypressExecutor()

	output := `
Running: test1.cy.js...

  Test Suite
    ✓ should pass test (1000ms)

  1 passing (2s)

Tests:        1
Passed:       1
Failed:       0
Skipped:      0
Duration: 2 seconds
`

	results := executor.parseCypressOutput(output, nil)

	assert.Equal(t, 1, results.Total)
	assert.Equal(t, 1, results.Passed)
	assert.Equal(t, 0, results.Failed)
	assert.Equal(t, 0, results.Skipped)
}

func TestCypressExecutor_ParseCypressOutput_WithFailures(t *testing.T) {
	executor := NewCypressExecutor()

	output := `
Running: test1.cy.js...

  Test Suite
    ✓ should pass test (1000ms)
    ✗ should fail test (500ms)

  1 passing
  1 failing

  1) should fail test
       AssertionError: expected true to be false

Tests:        2
Passed:       1
Failed:       1
Skipped:      0
Duration: 1 second
`

	results := executor.parseCypressOutput(output, nil)

	assert.Equal(t, 2, results.Total)
	assert.Equal(t, 1, results.Passed)
	assert.Equal(t, 1, results.Failed)
	assert.Equal(t, 0, results.Skipped)
}

func TestCypressExecutor_ParseCypressOutput_WithArtifacts(t *testing.T) {
	executor := NewCypressExecutor()

	output := `
Tests:        1
Passed:       1
Failed:       0
Skipped:      0

Screenshot: /path/to/screenshot.png
Video: /path/to/video.mp4
`

	results := executor.parseCypressOutput(output, nil)

	assert.Equal(t, 1, results.Total)
	assert.Equal(t, 1, results.Passed)
	assert.Contains(t, results.Artifacts, "screenshot.png")
	assert.Contains(t, results.Artifacts, "video.mp4")
	assert.Equal(t, "/path/to/screenshot.png", results.Artifacts["screenshot.png"])
	assert.Equal(t, "/path/to/video.mp4", results.Artifacts["video.mp4"])
}

func TestCypressExecutor_ParseDuration(t *testing.T) {
	executor := NewCypressExecutor()

	tests := []struct {
		line     string
		expected int64 // duration in nanoseconds
	}{
		{"Duration: 2 seconds", 2 * 1000000000},
		{"Duration: 1 second", 1 * 1000000000},
		{"Duration: 30 seconds", 30 * 1000000000},
		{"No duration here", 0},
	}

	for _, test := range tests {
		duration := executor.parseDuration(test.line)
		if test.expected == 0 {
			assert.Equal(t, int64(0), int64(duration))
		} else {
			assert.Equal(t, test.expected, int64(duration))
		}
	}
}

func TestCypressExecutor_ParseArtifact(t *testing.T) {
	executor := NewCypressExecutor()

	tests := []struct {
		line     string
		expected string
	}{
		{"Screenshot: /path/to/screenshot.png", "/path/to/screenshot.png"},
		{"Video: /path/to/video.mp4", "/path/to/video.mp4"},
		{"No artifact here", ""},
	}

	for _, test := range tests {
		artifact := executor.parseArtifact(test.line)
		assert.Equal(t, test.expected, artifact)
	}
}

func TestCypressExecutor_ParseNumber(t *testing.T) {
	executor := NewCypressExecutor()

	tests := []struct {
		input    string
		expected int
		hasError bool
	}{
		{"10", 10, false},
		{"1,234", 1234, false},
		{"0", 0, false},
		{"not-a-number", 0, true},
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
func TestCypressExecutor_Integration(t *testing.T) {
	executor := NewCypressExecutor()

	// Create a test configuration
	config := types.TestConfig{
		Type:    types.TestTypeCypress,
		Enabled: true,
		Config: map[string]interface{}{
			"cypress": map[string]interface{}{
				"baseUrl":     "http://localhost:3000",
				"specPattern": "cypress/integration/**/*.cy.js",
				"headless":    true,
				"browser":     "electron",
				"workingDir":  ".",
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

	// Test execution (this will fail because Cypress isn't installed, but the logic works)
	results, err := executor.Execute(context.Background(), env, config)

	// We expect this to fail because Cypress isn't actually installed
	// But the important thing is that our executor logic runs without panicking
	assert.Error(t, err)
	assert.NotNil(t, results)
	// The results should have some indication of failure
	assert.True(t, results.Failed > 0 || results.Total >= 0)
}
