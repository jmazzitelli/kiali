package api

import (
	"context"

	"github.com/kiali/kiali-integration-framework/internal/test"
	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
)

// TestAPI provides a public interface for test execution
type TestAPI interface {
	// Executor management
	CreateExecutor(testType types.TestType) (types.TestExecutorInterface, error)
	GetSupportedTestTypes() []types.TestType
	IsTestTypeSupported(testType types.TestType) bool

	// Test execution
	ExecuteTest(ctx context.Context, env *types.Environment, config types.TestConfig) (*types.TestResults, error)
	ExecuteTests(ctx context.Context, env *types.Environment, tests map[string]types.TestConfig) (map[string]*types.TestResults, error)

	// Multi-cluster test execution
	ExecuteMultiClusterTest(ctx context.Context, env *types.Environment, config types.MultiClusterTestConfig) (*types.MultiClusterTestResults, error)
	ExecuteMultiClusterTests(ctx context.Context, env *types.Environment, tests map[string]types.MultiClusterTestConfig) (map[string]*types.MultiClusterTestResults, error)

	// Test status and control
	CancelTest(ctx context.Context, testType types.TestType, executionID string) error
	GetTestStatus(ctx context.Context, testType types.TestType, executionID string) (types.TestStatus, error)

	// Multi-cluster test control
	CancelMultiClusterExecution(ctx context.Context, executionID string) error
	GetMultiClusterExecutionStatus(ctx context.Context, executionID string) (types.TestStatus, error)
	ListActiveMultiClusterExecutions(ctx context.Context) ([]string, error)
}

// testAPIImpl implements TestAPI
type testAPIImpl struct {
	factory                 *test.Factory
	multiClusterCoordinator types.MultiClusterTestCoordinatorInterface
}

// NewTestAPI creates a new test API instance
func NewTestAPI() TestAPI {
	return &testAPIImpl{
		factory:                 test.NewFactory(),
		multiClusterCoordinator: test.NewMultiClusterCoordinator(),
	}
}

// CreateExecutor creates a test executor
func (t *testAPIImpl) CreateExecutor(testType types.TestType) (types.TestExecutorInterface, error) {
	return t.factory.GetExecutor(testType)
}

// GetSupportedTestTypes returns supported test types
func (t *testAPIImpl) GetSupportedTestTypes() []types.TestType {
	return t.factory.GetSupportedTypes()
}

// IsTestTypeSupported checks if a test type is supported
func (t *testAPIImpl) IsTestTypeSupported(testType types.TestType) bool {
	return t.factory.IsSupported(testType)
}

// ExecuteTest executes a single test
func (t *testAPIImpl) ExecuteTest(ctx context.Context, env *types.Environment, config types.TestConfig) (*types.TestResults, error) {
	logger := utils.GetGlobalLogger()
	if logger != nil {
		logger.Infof("Executing test: %s (%s)", config.Type, config.Config)
	}

	// Get the executor
	executor, err := t.factory.GetExecutor(config.Type)
	if err != nil {
		return nil, err
	}

	// Validate configuration
	if err := executor.ValidateConfig(config); err != nil {
		return nil, err
	}

	// Execute the test
	results, err := executor.Execute(ctx, env, config)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ExecuteTests executes multiple tests
func (t *testAPIImpl) ExecuteTests(ctx context.Context, env *types.Environment, tests map[string]types.TestConfig) (map[string]*types.TestResults, error) {
	logger := utils.GetGlobalLogger()
	results := make(map[string]*types.TestResults)

	for testName, config := range tests {
		if !config.Enabled {
			if logger != nil {
				logger.Debugf("Skipping disabled test: %s", testName)
			}
			continue
		}

		if logger != nil {
			logger.Infof("Executing test: %s (%s)", testName, config.Type)
		}

		// Execute individual test
		testResults, err := t.ExecuteTest(ctx, env, config)
		if err != nil {
			if logger != nil {
				logger.Errorf("Test execution failed for %s: %v", testName, err)
			}
			return nil, err
		}

		results[testName] = testResults
	}

	return results, nil
}

// CancelTest cancels a running test
func (t *testAPIImpl) CancelTest(ctx context.Context, testType types.TestType, executionID string) error {
	executor, err := t.factory.GetExecutor(testType)
	if err != nil {
		return err
	}

	return executor.Cancel(ctx, executionID)
}

// GetTestStatus gets the status of a test execution
func (t *testAPIImpl) GetTestStatus(ctx context.Context, testType types.TestType, executionID string) (types.TestStatus, error) {
	executor, err := t.factory.GetExecutor(testType)
	if err != nil {
		return types.TestStatusFailed, err
	}

	return executor.GetStatus(ctx, executionID)
}

// ExecuteMultiClusterTest executes a single multi-cluster test
func (t *testAPIImpl) ExecuteMultiClusterTest(ctx context.Context, env *types.Environment, config types.MultiClusterTestConfig) (*types.MultiClusterTestResults, error) {
	logger := utils.GetGlobalLogger()
	if logger != nil {
		logger.Infof("Executing multi-cluster test: %s (%s)", config.Type, config.Config)
	}

	// Execute the multi-cluster test
	results, err := t.multiClusterCoordinator.ExecuteTests(ctx, env, config)
	if err != nil {
		if logger != nil {
			logger.Errorf("Multi-cluster test execution failed: %v", err)
		}
		return nil, err
	}

	return results, nil
}

// ExecuteMultiClusterTests executes multiple multi-cluster tests
func (t *testAPIImpl) ExecuteMultiClusterTests(ctx context.Context, env *types.Environment, tests map[string]types.MultiClusterTestConfig) (map[string]*types.MultiClusterTestResults, error) {
	logger := utils.GetGlobalLogger()
	results := make(map[string]*types.MultiClusterTestResults)

	for testName, config := range tests {
		if !config.Enabled {
			if logger != nil {
				logger.Debugf("Skipping disabled multi-cluster test: %s", testName)
			}
			continue
		}

		if logger != nil {
			logger.Infof("Executing multi-cluster test: %s (%s)", testName, config.Type)
		}

		// Execute individual multi-cluster test
		testResults, err := t.ExecuteMultiClusterTest(ctx, env, config)
		if err != nil {
			if logger != nil {
				logger.Errorf("Multi-cluster test execution failed for %s: %v", testName, err)
			}
			return nil, err
		}

		results[testName] = testResults
	}

	return results, nil
}

// CancelMultiClusterExecution cancels a multi-cluster test execution
func (t *testAPIImpl) CancelMultiClusterExecution(ctx context.Context, executionID string) error {
	return t.multiClusterCoordinator.CancelExecution(ctx, executionID)
}

// GetMultiClusterExecutionStatus gets the status of a multi-cluster test execution
func (t *testAPIImpl) GetMultiClusterExecutionStatus(ctx context.Context, executionID string) (types.TestStatus, error) {
	return t.multiClusterCoordinator.GetExecutionStatus(ctx, executionID)
}

// ListActiveMultiClusterExecutions lists all active multi-cluster test executions
func (t *testAPIImpl) ListActiveMultiClusterExecutions(ctx context.Context) ([]string, error) {
	return t.multiClusterCoordinator.ListActiveExecutions(ctx)
}
