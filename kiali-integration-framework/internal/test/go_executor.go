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

// GoExecutor implements the TestExecutorInterface for Go tests
type GoExecutor struct {
	logger *utils.Logger
}

// NewGoExecutor creates a new Go test executor
func NewGoExecutor() *GoExecutor {
	return &GoExecutor{
		logger: utils.GetGlobalLogger(),
	}
}

// Name returns the name of the executor
func (e *GoExecutor) Name() string {
	return "go"
}

// Type returns the test type
func (e *GoExecutor) Type() types.TestType {
	return types.TestTypeGo
}

// ValidateConfig validates the Go test configuration
func (e *GoExecutor) ValidateConfig(config types.TestConfig) error {
	if config.Type != types.TestTypeGo {
		return utils.NewFrameworkError(utils.ErrCodeInvalidParameter,
			fmt.Sprintf("invalid test type for Go executor: %s", config.Type), nil)
	}

	// Validate required configuration
	goConfig, ok := config.Config["go"].(map[string]interface{})
	if !ok {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"go configuration section is required", nil)
	}

	// Check for required fields - at least one of packages or package must be specified
	packages, hasPackages := goConfig["packages"].([]interface{})
	pkg, hasPkg := goConfig["package"].(string)

	if !hasPackages && !hasPkg {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"either 'packages' array or 'package' string must be specified in go configuration", nil)
	}

	// If packages is specified, ensure it's not empty
	if hasPackages && len(packages) == 0 {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"packages array cannot be empty", nil)
	}

	// If package is specified, ensure it's not empty
	if hasPkg && strings.TrimSpace(pkg) == "" {
		return utils.NewFrameworkError(utils.ErrCodeConfigInvalid,
			"package cannot be empty", nil)
	}

	return nil
}

// Execute runs the Go tests
func (e *GoExecutor) Execute(ctx context.Context, env *types.Environment, config types.TestConfig) (*types.TestResults, error) {
	// Validate configuration first
	if err := e.ValidateConfig(config); err != nil {
		return nil, err
	}

	// Extract Go configuration
	goConfig, _ := config.Config["go"].(map[string]interface{})

	// Prepare execution context
	startTime := time.Now()

	// Build Go test command
	cmd, args := e.buildGoCommand(goConfig)

	// Set up environment variables
	envVars := e.buildEnvironmentVariables(goConfig, env)

	// Execute Go tests within logging operation
	var results *types.TestResults
	err := e.logger.LogOperationWithContext("execute go tests", map[string]interface{}{
		"testType": config.Type,
		"config":   config.Config,
	}, func() error {
		e.logger.Info("Starting Go test execution")
		e.logger.Infof("Running command: %s %s", cmd, strings.Join(args, " "))

		execCmd := exec.CommandContext(ctx, cmd, args...)
		execCmd.Env = append(os.Environ(), envVars...)
		execCmd.Dir = e.getWorkingDirectory(goConfig)

		// Capture output
		output, execErr := execCmd.CombinedOutput()
		endTime := time.Now()

		// Parse results
		results = e.parseGoOutput(string(output), execErr)

		// Update timing
		results.Duration = endTime.Sub(startTime)

		e.logger.Infof("Go execution completed in %v", results.Duration)

		// Return the execution error to propagate it up
		return execErr
	})

	// Return results and any error from the operation
	return results, err
}

// buildGoCommand builds the Go test command and arguments
func (e *GoExecutor) buildGoCommand(config map[string]interface{}) (string, []string) {
	cmd := "go"
	args := []string{"test"}

	// Add packages or package
	if packages, ok := config["packages"].([]interface{}); ok && len(packages) > 0 {
		// Convert interface{} slice to string slice
		pkgStrings := make([]string, len(packages))
		for i, pkg := range packages {
			if strPkg, ok := pkg.(string); ok {
				pkgStrings[i] = strPkg
			}
		}
		args = append(args, pkgStrings...)
	} else if pkg, ok := config["package"].(string); ok && pkg != "" {
		args = append(args, pkg)
	}

	// Add verbosity flag
	if verbose, ok := config["verbose"].(bool); ok && verbose {
		args = append(args, "-v")
	}

	// Add timeout
	if timeout, ok := config["timeout"].(string); ok && timeout != "" {
		args = append(args, "-timeout", timeout)
	}

	// Add test count
	if count, ok := config["count"].(int); ok && count > 0 {
		args = append(args, "-count", fmt.Sprintf("%d", count))
	}

	// Add race detection
	if race, ok := config["race"].(bool); ok && race {
		args = append(args, "-race")
	}

	// Add coverage
	if coverage, ok := config["coverage"].(bool); ok && coverage {
		args = append(args, "-cover")
		if coverageProfile, ok := config["coverageProfile"].(string); ok && coverageProfile != "" {
			args = append(args, "-coverprofile", coverageProfile)
		}
	}

	// Add build tags
	if tags, ok := config["tags"].([]interface{}); ok && len(tags) > 0 {
		tagStrings := make([]string, len(tags))
		for i, tag := range tags {
			if strTag, ok := tag.(string); ok {
				tagStrings[i] = strTag
			}
		}
		if len(tagStrings) > 0 {
			args = append(args, "-tags", strings.Join(tagStrings, ","))
		}
	}

	// Add run pattern
	if run, ok := config["run"].(string); ok && run != "" {
		args = append(args, "-run", run)
	}

	// Add skip pattern
	if skip, ok := config["skip"].(string); ok && skip != "" {
		args = append(args, "-skip", skip)
	}

	// Add short flag
	if short, ok := config["short"].(bool); ok && short {
		args = append(args, "-short")
	}

	// Add benchmark flags
	if bench, ok := config["bench"].(string); ok && bench != "" {
		args = append(args, "-bench", bench)
		if benchTime, ok := config["benchTime"].(string); ok && benchTime != "" {
			args = append(args, "-benchtime", benchTime)
		}
	}

	return cmd, args
}

// buildEnvironmentVariables builds environment variables for Go tests
func (e *GoExecutor) buildEnvironmentVariables(config map[string]interface{}, env *types.Environment) []string {
	envVars := []string{}

	// Add custom environment variables
	if envMap, ok := config["env"].(map[string]interface{}); ok {
		for key, value := range envMap {
			if strValue, ok := value.(string); ok {
				envVars = append(envVars, fmt.Sprintf("%s=%s", key, strValue))
			}
		}
	}

	// Add multi-cluster environment variables if available
	if env != nil {
		// Primary cluster info
		envVars = append(envVars, fmt.Sprintf("KIALI_INT_CLUSTER_PROVIDER=%s", env.Cluster.Provider))
		envVars = append(envVars, fmt.Sprintf("KIALI_INT_CLUSTER_NAME=%s", env.Cluster.Name))

		// Component information
		for componentName, componentConfig := range env.Components {
			if componentConfig.Enabled {
				envVars = append(envVars, fmt.Sprintf("KIALI_INT_COMPONENT_%s_ENABLED=true", strings.ToUpper(componentName)))
				envVars = append(envVars, fmt.Sprintf("KIALI_INT_COMPONENT_%s_TYPE=%s", strings.ToUpper(componentName), componentConfig.Type))
				if componentConfig.Version != "" {
					envVars = append(envVars, fmt.Sprintf("KIALI_INT_COMPONENT_%s_VERSION=%s", strings.ToUpper(componentName), componentConfig.Version))
				}
			}
		}
	}

	return envVars
}

// getWorkingDirectory returns the working directory for Go test execution
func (e *GoExecutor) getWorkingDirectory(config map[string]interface{}) string {
	if workingDir, ok := config["workingDir"].(string); ok && workingDir != "" {
		return workingDir
	}

	// Default to current directory
	if wd, err := os.Getwd(); err == nil {
		return wd
	}

	return "."
}

// parseGoOutput parses the Go test command output to extract test results
func (e *GoExecutor) parseGoOutput(output string, cmdErr error) *types.TestResults {
	results := &types.TestResults{
		Artifacts: make(map[string]string),
	}

	// Parse the output for test results
	lines := strings.Split(output, "\n")

	// First pass: collect summary information and artifacts
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for test results summary
		if strings.Contains(line, "PASS") || strings.Contains(line, "FAIL") ||
			strings.Contains(line, "SKIP") || strings.Contains(line, "RUN") {

			// Parse summary lines like "PASS: 5, FAIL: 1, SKIP: 0"
			if strings.Contains(line, "PASS:") || strings.Contains(line, "FAIL:") || strings.Contains(line, "SKIP:") {
				e.parseGoSummaryLine(line, results)
			}
		}

		// Look for coverage information
		if strings.Contains(line, "coverage:") {
			if coverageFile := e.parseCoverageArtifact(line); coverageFile != "" {
				results.Artifacts[filepath.Base(coverageFile)] = coverageFile
			}
		}

		// Look for benchmark results
		if strings.Contains(line, "Benchmark") && strings.Contains(line, "\t") {
			// Could extend to parse benchmark results in the future
		}
	}

	// If we couldn't parse results from output, try to infer from error
	if cmdErr != nil && results.Total == 0 {
		// Check if there were any test runs by looking for test execution patterns
		testRunCount := strings.Count(output, "=== RUN")
		if testRunCount > 0 {
			results.Total = testRunCount
			// Assume all failed if we have an error and no clear results
			results.Failed = testRunCount
		} else {
			// No tests were run, mark as failed
			results.Failed = 1
		}
		results.Total = results.Passed + results.Failed + results.Skipped
	} else {
		results.Total = results.Passed + results.Failed + results.Skipped
	}

	return results
}

// parseGoSummaryLine parses a Go test summary line
func (e *GoExecutor) parseGoSummaryLine(line string, results *types.TestResults) {
	// Handle different formats:
	// "PASS: 5, FAIL: 1, SKIP: 0"
	// "ok  	package/name	0.123s"
	// "FAIL	package/name	0.123s"

	// Check for detailed summary format
	if strings.Contains(line, "PASS:") || strings.Contains(line, "FAIL:") || strings.Contains(line, "SKIP:") {
		parts := strings.Split(line, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "PASS:") {
				if val, err := e.parseNumber(strings.TrimPrefix(part, "PASS:")); err == nil {
					results.Passed = val
				}
			} else if strings.HasPrefix(part, "FAIL:") {
				if val, err := e.parseNumber(strings.TrimPrefix(part, "FAIL:")); err == nil {
					results.Failed = val
				}
			} else if strings.HasPrefix(part, "SKIP:") {
				if val, err := e.parseNumber(strings.TrimPrefix(part, "SKIP:")); err == nil {
					results.Skipped = val
				}
			}
		}
	}

	// Check for package-level results
	if strings.HasPrefix(line, "ok") || strings.HasPrefix(line, "FAIL") {
		// This is a package-level result, we could count these
		// For now, we'll let the detailed parsing handle it
	}
}

// parseCoverageArtifact extracts coverage file path from output
func (e *GoExecutor) parseCoverageArtifact(line string) string {
	// Look for coverage file in the line
	// This is a simple implementation - could be enhanced
	parts := strings.Fields(line)
	for _, part := range parts {
		if strings.HasSuffix(part, ".out") || strings.Contains(part, "coverage") {
			return strings.Trim(part, "[]()")
		}
	}
	return ""
}

// parseNumber parses a string to int (helper function)
func (e *GoExecutor) parseNumber(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &result)
	return result, err
}

// Cancel cancels a running Go test execution
func (e *GoExecutor) Cancel(ctx context.Context, executionID string) error {
	// Note: Go doesn't have built-in test cancellation like Cypress
	// This would require process management or signal handling
	return utils.NewFrameworkError(utils.ErrCodeInternalError,
		"Go test cancellation not yet implemented - requires process management", nil)
}

// GetStatus returns the status of a Go test execution
func (e *GoExecutor) GetStatus(ctx context.Context, executionID string) (types.TestStatus, error) {
	// Note: Go test execution status tracking would require
	// maintaining process state or using external tools
	return types.TestStatusRunning, utils.NewFrameworkError(utils.ErrCodeInternalError,
		"Go test status checking not yet implemented - requires process tracking", nil)
}
