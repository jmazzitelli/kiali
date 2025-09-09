package test

import (
	"context"
	"fmt"

	"github.com/kiali/kiali-integration-framework/pkg/types"
	"github.com/kiali/kiali-integration-framework/pkg/utils"
)

// Factory manages test executors
type Factory struct {
	executors map[types.TestType]types.TestExecutorInterface
	logger    *utils.Logger
}

// NewFactory creates a new test executor factory
func NewFactory() *Factory {
	factory := &Factory{
		executors: make(map[types.TestType]types.TestExecutorInterface),
		logger:    utils.GetGlobalLogger(),
	}

	// Register default executors
	factory.registerExecutors()

	return factory
}

// registerExecutors registers all available test executors
func (f *Factory) registerExecutors() {
	// Register Cypress executor
	cypressExecutor := NewCypressExecutor()
	f.executors[types.TestTypeCypress] = cypressExecutor
	f.logger.Debugf("Registered test executor: %s", cypressExecutor.Name())

	// Register Go executor
	goExecutor := NewGoExecutor()
	f.executors[types.TestTypeGo] = goExecutor
	f.logger.Debugf("Registered test executor: %s", goExecutor.Name())
}

// GetExecutor returns the test executor for the specified test type
func (f *Factory) GetExecutor(testType types.TestType) (types.TestExecutorInterface, error) {
	executor, exists := f.executors[testType]
	if !exists {
		return nil, utils.NewFrameworkError(utils.ErrCodeInvalidParameter,
			fmt.Sprintf("no executor registered for test type: %s", testType), nil)
	}

	f.logger.Debugf("Retrieved executor for test type: %s", testType)
	return executor, nil
}

// GetSupportedTypes returns all supported test types
func (f *Factory) GetSupportedTypes() []types.TestType {
	types := make([]types.TestType, 0, len(f.executors))
	for testType := range f.executors {
		types = append(types, testType)
	}
	return types
}

// IsSupported checks if a test type is supported
func (f *Factory) IsSupported(testType types.TestType) bool {
	_, exists := f.executors[testType]
	return exists
}

// RegisterExecutor registers a custom test executor
func (f *Factory) RegisterExecutor(executor types.TestExecutorInterface) error {
	if executor == nil {
		return utils.NewFrameworkError(utils.ErrCodeInvalidParameter,
			"executor cannot be nil", nil)
	}

	testType := executor.Type()
	if f.IsSupported(testType) {
		f.logger.Warnf("Overriding existing executor for test type: %s", testType)
	}

	f.executors[testType] = executor
	f.logger.Infof("Registered custom test executor: %s (%s)", executor.Name(), testType)

	return nil
}

// ExecuteTest executes a test using the appropriate executor
func (f *Factory) ExecuteTest(ctx context.Context, env *types.Environment, config types.TestConfig) (*types.TestResults, error) {
	// Get the executor
	executor, err := f.GetExecutor(config.Type)
	if err != nil {
		return nil, err
	}

	// Execute the test
	return executor.Execute(ctx, env, config)
}
