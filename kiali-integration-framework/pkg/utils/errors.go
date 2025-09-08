package utils

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// FrameworkError represents a custom error type for the framework
type FrameworkError struct {
	Code    ErrorCode              `json:"code"`
	Message string                 `json:"message"`
	Cause   error                  `json:"cause,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// ErrorCode represents different types of errors
type ErrorCode string

const (
	// Configuration errors
	ErrCodeConfigInvalid     ErrorCode = "CONFIG_INVALID"
	ErrCodeConfigNotFound    ErrorCode = "CONFIG_NOT_FOUND"
	ErrCodeConfigParseFailed ErrorCode = "CONFIG_PARSE_FAILED"

	// Cluster errors
	ErrCodeClusterCreateFailed ErrorCode = "CLUSTER_CREATE_FAILED"
	ErrCodeClusterDeleteFailed ErrorCode = "CLUSTER_DELETE_FAILED"
	ErrCodeClusterNotFound     ErrorCode = "CLUSTER_NOT_FOUND"
	ErrCodeClusterUnhealthy    ErrorCode = "CLUSTER_UNHEALTHY"

	// Component errors
	ErrCodeComponentInstallFailed   ErrorCode = "COMPONENT_INSTALL_FAILED"
	ErrCodeComponentUninstallFailed ErrorCode = "COMPONENT_UNINSTALL_FAILED"
	ErrCodeComponentUpdateFailed    ErrorCode = "COMPONENT_UPDATE_FAILED"
	ErrCodeComponentNotFound        ErrorCode = "COMPONENT_NOT_FOUND"
	ErrCodeComponentAlreadyExists   ErrorCode = "COMPONENT_ALREADY_EXISTS"

	// Test errors
	ErrCodeTestExecutionFailed ErrorCode = "TEST_EXECUTION_FAILED"
	ErrCodeTestTimeout         ErrorCode = "TEST_TIMEOUT"
	ErrCodeTestSetupFailed     ErrorCode = "TEST_SETUP_FAILED"

	// Validation errors
	ErrCodeValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrCodeInvalidParameter ErrorCode = "INVALID_PARAMETER"

	// System errors
	ErrCodeCommandFailed    ErrorCode = "COMMAND_FAILED"
	ErrCodeNetworkError     ErrorCode = "NETWORK_ERROR"
	ErrCodeFileSystemError  ErrorCode = "FILESYSTEM_ERROR"
	ErrCodePermissionDenied ErrorCode = "PERMISSION_DENIED"

	// Generic errors
	ErrCodeInternalError ErrorCode = "INTERNAL_ERROR"
	ErrCodeUnknownError  ErrorCode = "UNKNOWN_ERROR"
)

// Error implements the error interface
func (e *FrameworkError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *FrameworkError) Unwrap() error {
	return e.Cause
}

// NewFrameworkError creates a new FrameworkError
func NewFrameworkError(code ErrorCode, message string, cause error) *FrameworkError {
	return &FrameworkError{
		Code:    code,
		Message: message,
		Cause:   cause,
		Context: make(map[string]interface{}),
	}
}

// NewFrameworkErrorWithContext creates a new FrameworkError with context
func NewFrameworkErrorWithContext(code ErrorCode, message string, cause error, context map[string]interface{}) *FrameworkError {
	err := NewFrameworkError(code, message, cause)
	err.Context = context
	return err
}

// IsFrameworkError checks if an error is a FrameworkError
func IsFrameworkError(err error) bool {
	_, ok := err.(*FrameworkError)
	return ok
}

// GetErrorCode returns the error code from a FrameworkError
func GetErrorCode(err error) ErrorCode {
	if fwErr, ok := err.(*FrameworkError); ok {
		return fwErr.Code
	}
	return ErrCodeUnknownError
}

// WrapError wraps an error with additional context
func WrapError(err error, code ErrorCode, message string) error {
	if err == nil {
		return nil
	}

	return &FrameworkError{
		Code:    code,
		Message: message,
		Cause:   err,
		Context: make(map[string]interface{}),
	}
}

// WrapErrorWithContext wraps an error with context
func WrapErrorWithContext(err error, code ErrorCode, message string, context map[string]interface{}) error {
	if err == nil {
		return nil
	}

	return &FrameworkError{
		Code:    code,
		Message: message,
		Cause:   err,
		Context: context,
	}
}

// HandleCommandError handles errors from command execution
func HandleCommandError(cmd *exec.Cmd, err error) error {
	if err == nil {
		return nil
	}

	// Extract command details
	command := strings.Join(cmd.Args, " ")
	context := map[string]interface{}{
		"command": command,
		"dir":     cmd.Dir,
	}

	// Check for specific error types
	if exitErr, ok := err.(*exec.ExitError); ok {
		context["exit_code"] = exitErr.ExitCode()
		context["stderr"] = string(exitErr.Stderr)
		return NewFrameworkErrorWithContext(ErrCodeCommandFailed, "Command execution failed", err, context)
	}

	return NewFrameworkErrorWithContext(ErrCodeCommandFailed, "Failed to execute command", err, context)
}

// HandleNetworkError handles network-related errors
func HandleNetworkError(err error, operation string) error {
	if err == nil {
		return nil
	}

	context := map[string]interface{}{
		"operation": operation,
	}

	// Check for specific network error types
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			context["error_type"] = "timeout"
		} else {
			context["error_type"] = "network"
		}
	}

	return NewFrameworkErrorWithContext(ErrCodeNetworkError, "Network operation failed", err, context)
}

// HandleValidationError creates a validation error
func HandleValidationError(field string, value interface{}, reason string) error {
	context := map[string]interface{}{
		"field":  field,
		"value":  value,
		"reason": reason,
	}

	message := fmt.Sprintf("Validation failed for field '%s': %s", field, reason)
	return NewFrameworkErrorWithContext(ErrCodeValidationFailed, message, nil, context)
}

// HandleFileSystemError handles file system related errors
func HandleFileSystemError(err error, operation string, path string) error {
	if err == nil {
		return nil
	}

	context := map[string]interface{}{
		"operation": operation,
		"path":      path,
	}

	return NewFrameworkErrorWithContext(ErrCodeFileSystemError, "File system operation failed", err, context)
}

// IsRetryableError determines if an error is retryable
func IsRetryableError(err error) bool {
	if fwErr, ok := err.(*FrameworkError); ok {
		switch fwErr.Code {
		case ErrCodeNetworkError, ErrCodeClusterUnhealthy, ErrCodeCommandFailed:
			return true
		default:
			return false
		}
	}

	// Check for network errors that are typically retryable
	if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
		return true
	}

	return false
}

// GetErrorSeverity returns the severity level of an error
func GetErrorSeverity(err error) string {
	if fwErr, ok := err.(*FrameworkError); ok {
		switch fwErr.Code {
		case ErrCodeInternalError, ErrCodeUnknownError:
			return "critical"
		case ErrCodeCommandFailed, ErrCodeClusterCreateFailed, ErrCodeClusterDeleteFailed:
			return "high"
		case ErrCodeConfigInvalid, ErrCodeValidationFailed, ErrCodeNetworkError:
			return "medium"
		case ErrCodeConfigNotFound, ErrCodeComponentNotFound, ErrCodeClusterNotFound:
			return "low"
		default:
			return "medium"
		}
	}

	return "medium"
}
