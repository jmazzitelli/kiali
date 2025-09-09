package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
)

// CypressExecutor implements the TestExecutorInterface for Cypress tests
type CypressExecutor struct {
	logger *utils.Logger
}

// NewCypressExecutor creates a new Cypress test executor
func NewCypressExecutor() *CypressExecutor {
	return &CypressExecutor{
		logger: utils.GetGlobalLogger(),
	}
}

// Name returns the name of the executor
func (e *CypressExecutor) Name() string {
	return "cypress"
}

// Type returns the test type
func (e *CypressExecutor) Type() types.TestType {
	return types.TestTypeCypress
}

// ValidateConfig validates the Cypress test configuration
func (e *CypressExecutor) ValidateConfig(config types.TestConfig) error {
	if config.Type != types.TestTypeCypress {
		return utils.NewFrameworkError(utils.ErrCodeInvalidParameter,
			fmt.Sprintf("invalid test type for Cypress executor: %s", config.Type), nil)
	}

	// Validate required configuration
	cypressConfig, ok := config.Config["cypress"].(map[string]interface{})
	if !ok {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"cypress configuration section is required", nil)
	}

	// Check for required fields
	if baseURL, exists := cypressConfig["baseUrl"]; !exists || baseURL == "" {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"baseUrl is required in cypress configuration", nil)
	}

	if specPattern, exists := cypressConfig["specPattern"]; !exists || specPattern == "" {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"specPattern is required in cypress configuration", nil)
	}

	return nil
}

// Execute runs the Cypress tests
func (e *CypressExecutor) Execute(ctx context.Context, env *types.Environment, config types.TestConfig) (*types.TestResults, error) {
	// Validate configuration first
	if err := e.ValidateConfig(config); err != nil {
		return nil, err
	}

	// Extract Cypress configuration
	cypressConfig, _ := config.Config["cypress"].(map[string]interface{})

	// Prepare execution context
	startTime := time.Now()

	// Build Cypress command
	cmd, args := e.buildCypressCommand(cypressConfig)

	// Set up environment variables
	envVars := e.buildEnvironmentVariables(cypressConfig)

	// Execute Cypress tests within logging operation
	var results *types.TestResults
	err := e.logger.LogOperationWithContext("execute cypress tests", map[string]interface{}{
		"testType": config.Type,
		"config":   config.Config,
	}, func() error {
		e.logger.Info("Starting Cypress test execution")
		e.logger.Infof("Running command: %s %s", cmd, strings.Join(args, " "))

		execCmd := exec.CommandContext(ctx, cmd, args...)
		execCmd.Env = append(os.Environ(), envVars...)
		execCmd.Dir = e.getWorkingDirectory(cypressConfig)

		// Capture output
		output, execErr := execCmd.CombinedOutput()
		endTime := time.Now()

		// Parse results
		results = e.parseCypressOutput(string(output), execErr)

		// Update timing
		results.Duration = endTime.Sub(startTime)

		e.logger.Infof("Cypress execution completed in %v", results.Duration)

		// Return the execution error to propagate it up
		return execErr
	})

	// Return results and any error from the operation
	return results, err
}

// buildCypressCommand builds the Cypress command and arguments
func (e *CypressExecutor) buildCypressCommand(config map[string]interface{}) (string, []string) {
	// Default to npx cypress run
	cmd := "npx"
	args := []string{"cypress", "run"}

	// Add spec pattern if specified
	if specPattern, ok := config["specPattern"].(string); ok && specPattern != "" {
		args = append(args, "--spec", specPattern)
	}

	// Add browser if specified
	if browser, ok := config["browser"].(string); ok && browser != "" {
		args = append(args, "--browser", browser)
	}

	// Add headless mode (default true for CI)
	if headless, ok := config["headless"].(bool); ok && !headless {
		args = append(args, "--headed")
	} else {
		args = append(args, "--headless")
	}

	// Add record mode if configured
	if record, ok := config["record"].(bool); ok && record {
		args = append(args, "--record")
		if recordKey, ok := config["recordKey"].(string); ok && recordKey != "" {
			args = append(args, "--key", recordKey)
		}
	}

	// Add config options
	configOpts := []string{}
	if baseURL, ok := config["baseUrl"].(string); ok && baseURL != "" {
		configOpts = append(configOpts, fmt.Sprintf("baseUrl=%s", baseURL))
	}
	if timeout, ok := config["defaultCommandTimeout"].(int); ok && timeout > 0 {
		configOpts = append(configOpts, fmt.Sprintf("defaultCommandTimeout=%d", timeout))
	}
	if requestTimeout, ok := config["requestTimeout"].(int); ok && requestTimeout > 0 {
		configOpts = append(configOpts, fmt.Sprintf("requestTimeout=%d", requestTimeout))
	}

	if len(configOpts) > 0 {
		args = append(args, "--config", strings.Join(configOpts, ","))
	}

	// Add environment variables
	envVars := []string{}
	if envFile, ok := config["env"].(map[string]interface{}); ok {
		for key, value := range envFile {
			if strValue, ok := value.(string); ok {
				envVars = append(envVars, fmt.Sprintf("%s=%s", key, strValue))
			}
		}
	}

	if len(envVars) > 0 {
		args = append(args, "--env", strings.Join(envVars, ","))
	}

	// Add tags if specified
	if tags, ok := config["tags"].([]interface{}); ok && len(tags) > 0 {
		tagStrings := make([]string, len(tags))
		for i, tag := range tags {
			if strTag, ok := tag.(string); ok {
				tagStrings[i] = strTag
			}
		}
		if len(tagStrings) > 0 {
			args = append(args, "--grep", strings.Join(tagStrings, "|"))
		}
	}

	// Add reporter options
	if reporter, ok := config["reporter"].(string); ok && reporter != "" {
		args = append(args, "--reporter", reporter)
		if reporterOptions, ok := config["reporterOptions"].(string); ok && reporterOptions != "" {
			args = append(args, "--reporter-options", reporterOptions)
		}
	}

	return cmd, args
}

// buildEnvironmentVariables builds environment variables for Cypress
func (e *CypressExecutor) buildEnvironmentVariables(config map[string]interface{}) []string {
	envVars := []string{}

	// Add base URL
	if baseURL, ok := config["baseUrl"].(string); ok && baseURL != "" {
		envVars = append(envVars, fmt.Sprintf("CYPRESS_BASE_URL=%s", baseURL))
	}

	// Add custom environment variables
	if env, ok := config["env"].(map[string]interface{}); ok {
		for key, value := range env {
			if strValue, ok := value.(string); ok {
				envVars = append(envVars, fmt.Sprintf("CYPRESS_%s=%s", strings.ToUpper(key), strValue))
			}
		}
	}

	return envVars
}

// getWorkingDirectory returns the working directory for Cypress execution
func (e *CypressExecutor) getWorkingDirectory(config map[string]interface{}) string {
	if workingDir, ok := config["workingDir"].(string); ok && workingDir != "" {
		return workingDir
	}

	// Default to current directory
	if wd, err := os.Getwd(); err == nil {
		return wd
	}

	return "."
}

// parseCypressOutput parses the Cypress command output to extract test results
func (e *CypressExecutor) parseCypressOutput(output string, cmdErr error) *types.TestResults {
	results := &types.TestResults{
		Artifacts: make(map[string]string),
	}

	// Parse the output for test results
	lines := strings.Split(output, "\n")

	// First pass: collect all summary lines
	var summaryLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for test result patterns
		if strings.Contains(line, "Tests:") || strings.Contains(line, "Passed:") ||
			strings.Contains(line, "Failed:") || strings.Contains(line, "Skipped:") {
			summaryLines = append(summaryLines, line)
		}

		// Look for duration
		if strings.Contains(line, "Duration:") {
			if duration := e.parseDuration(line); duration > 0 {
				results.Duration = duration
			}
		}

		// Look for screenshots/videos
		if strings.Contains(line, "Screenshot") || strings.Contains(line, "Video") {
			if artifact := e.parseArtifact(line); artifact != "" {
				// Store artifact path
				results.Artifacts[filepath.Base(artifact)] = artifact
			}
		}
	}

	// Parse summary lines
	if len(summaryLines) > 0 {
		summaryText := strings.Join(summaryLines, "\n")
		if err := e.parseSummaryLine(summaryText, results); err != nil {
			e.logger.Warnf("Failed to parse summary lines: %v", err)
		}
	}

	// Determine overall status
	if cmdErr != nil {
		if results.Total == 0 {
			results.Failed = 1
		}
		results.Total = results.Passed + results.Failed + results.Skipped
		e.logger.Warnf("Cypress execution failed: %v", cmdErr)
	} else {
		results.Total = results.Passed + results.Failed + results.Skipped
	}

	return results
}

// parseSummaryLine parses a Cypress summary line
func (e *CypressExecutor) parseSummaryLine(line string, results *types.TestResults) error {
	// Handle different formats:
	// "Tests: 10 Passed: 8 Failed: 2 Skipped: 0"
	// "Tests:        10"
	// "Passed:       8"
	// "Failed:       2"
	// "Skipped:      0"

	// Try multi-line format first (each metric on separate line)
	lines := strings.Split(line, "\n")
	if len(lines) > 1 {
		for _, singleLine := range lines {
			e.parseSingleSummaryLine(singleLine, results)
		}
		return nil
	}

	// Try single-line format
	return e.parseSingleSummaryLine(line, results)
}

// parseSingleSummaryLine parses a single summary line
func (e *CypressExecutor) parseSingleSummaryLine(line string, results *types.TestResults) error {
	line = strings.TrimSpace(line)

	if strings.Contains(line, "Tests:") && strings.Contains(line, "Passed:") {
		// Multi-metric single line
		parts := strings.Fields(line)
		for i, part := range parts {
			switch part {
			case "Tests:":
				if i+1 < len(parts) {
					if total, err := e.parseNumber(parts[i+1]); err == nil {
						results.Total = total
					}
				}
			case "Passed:":
				if i+1 < len(parts) {
					if passed, err := e.parseNumber(parts[i+1]); err == nil {
						results.Passed = passed
					}
				}
			case "Failed:":
				if i+1 < len(parts) {
					if failed, err := e.parseNumber(parts[i+1]); err == nil {
						results.Failed = failed
					}
				}
			case "Skipped:":
				if i+1 < len(parts) {
					if skipped, err := e.parseNumber(parts[i+1]); err == nil {
						results.Skipped = skipped
					}
				}
			}
		}
	} else {
		// Single metric line
		if strings.HasPrefix(line, "Tests:") {
			if total, err := e.parseNumber(strings.TrimSpace(strings.TrimPrefix(line, "Tests:"))); err == nil {
				results.Total = total
			}
		} else if strings.HasPrefix(line, "Passed:") {
			if passed, err := e.parseNumber(strings.TrimSpace(strings.TrimPrefix(line, "Passed:"))); err == nil {
				results.Passed = passed
			}
		} else if strings.HasPrefix(line, "Failed:") {
			if failed, err := e.parseNumber(strings.TrimSpace(strings.TrimPrefix(line, "Failed:"))); err == nil {
				results.Failed = failed
			}
		} else if strings.HasPrefix(line, "Skipped:") {
			if skipped, err := e.parseNumber(strings.TrimSpace(strings.TrimPrefix(line, "Skipped:"))); err == nil {
				results.Skipped = skipped
			}
		}
	}

	return nil
}

// parseDuration parses duration from output
func (e *CypressExecutor) parseDuration(line string) time.Duration {
	// Look for patterns like "Duration: 1 minute, 30 seconds" or "Duration: 30 seconds"
	if !strings.Contains(line, "Duration:") {
		return 0
	}

	// Simple parsing - could be enhanced for more complex formats
	if strings.Contains(line, "second") {
		// Extract number before "second"
		parts := strings.Fields(line)
		for i, part := range parts {
			if part == "second" || part == "seconds" {
				if i > 0 {
					if seconds, err := e.parseNumber(parts[i-1]); err == nil {
						return time.Duration(seconds) * time.Second
					}
				}
			}
		}
	}

	return 0
}

// parseArtifact extracts artifact path from output line
func (e *CypressExecutor) parseArtifact(line string) string {
	// Look for file paths in the line
	if strings.Contains(line, ".png") || strings.Contains(line, ".mp4") {
		// Simple extraction - look for path-like strings
		parts := strings.Fields(line)
		for _, part := range parts {
			if strings.Contains(part, ".png") || strings.Contains(part, ".mp4") {
				return strings.Trim(part, "[]()")
			}
		}
	}
	return ""
}

// parseNumber parses a string to int
func (e *CypressExecutor) parseNumber(s string) (int, error) {
	// Remove commas and parse
	clean := strings.ReplaceAll(s, ",", "")
	var result int
	_, err := fmt.Sscanf(clean, "%d", &result)
	return result, err
}

// Cancel cancels a running Cypress test execution
func (e *CypressExecutor) Cancel(ctx context.Context, executionID string) error {
	return utils.NewFrameworkError(utils.ErrCodeInternalError,
		"Cypress test cancellation not yet implemented", nil)
}

// GetStatus returns the status of a Cypress test execution
func (e *CypressExecutor) GetStatus(ctx context.Context, executionID string) (types.TestStatus, error) {
	return types.TestStatusRunning, utils.NewFrameworkError(utils.ErrCodeInternalError,
		"Cypress test status checking not yet implemented", nil)
}
