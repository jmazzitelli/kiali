package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// Logger wraps logrus.Logger with additional functionality
type Logger struct {
	*logrus.Logger
}

// LogLevel represents the logging level
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
	LogLevelPanic LogLevel = "panic"
)

// NewLogger creates a new logger instance
func NewLogger(level LogLevel, verbose bool) *Logger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// Set log level
	switch strings.ToLower(string(level)) {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	case "fatal":
		logger.SetLevel(logrus.FatalLevel)
	case "panic":
		logger.SetLevel(logrus.PanicLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}

	// Enable verbose mode (includes caller info)
	if verbose {
		logger.SetReportCaller(true)
	}

	return &Logger{Logger: logger}
}

// SetOutput sets the output destination for the logger
func (l *Logger) SetOutput(output io.Writer) {
	l.Logger.SetOutput(output)
}

// SetLogFile sets up logging to a file
func (l *Logger) SetLogFile(filename string) error {
	// Create log directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Set output to both file and stdout if verbose
	if l.Logger.GetLevel() == logrus.DebugLevel {
		l.Logger.SetOutput(io.MultiWriter(os.Stdout, file))
	} else {
		l.Logger.SetOutput(file)
	}

	return nil
}

// WithFields creates an entry from the logger with the provided fields
func (l *Logger) WithFields(fields logrus.Fields) *logrus.Entry {
	return l.Logger.WithFields(fields)
}

// WithField creates an entry from the logger with the provided field
func (l *Logger) WithField(key string, value interface{}) *logrus.Entry {
	return l.Logger.WithField(key, value)
}

// WithError creates an entry from the logger with the provided error
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// Debugf logs a message at level Debug on the standard logger.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Logger.Debugf(format, args...)
}

// Infof logs a message at level Info on the standard logger.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Logger.Infof(format, args...)
}

// Warnf logs a message at level Warn on the standard logger.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Logger.Warnf(format, args...)
}

// Errorf logs a message at level Error on the standard logger.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Logger.Errorf(format, args...)
}

// Fatalf logs a message at level Fatal on the standard logger.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Logger.Fatalf(format, args...)
}

// Panicf logs a message at level Panic on the standard logger.
func (l *Logger) Panicf(format string, args ...interface{}) {
	l.Logger.Panicf(format, args...)
}

// LogOperation logs the start and end of an operation with timing
func (l *Logger) LogOperation(operation string, fn func() error) error {
	l.Infof("Starting operation: %s", operation)
	err := fn()
	if err != nil {
		l.Errorf("Operation failed: %s - %v", operation, err)
		return err
	}
	l.Infof("Operation completed successfully: %s", operation)
	return nil
}

// LogOperationWithContext logs an operation with additional context
func (l *Logger) LogOperationWithContext(operation string, context map[string]interface{}, fn func() error) error {
	entry := l.WithFields(logrus.Fields(context))
	entry.Infof("Starting operation: %s", operation)

	err := fn()
	if err != nil {
		entry.Errorf("Operation failed: %s - %v", operation, err)
		return err
	}

	entry.Infof("Operation completed successfully: %s", operation)
	return nil
}

// Global logger instance
var globalLogger *Logger

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(logger *Logger) {
	globalLogger = logger
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() *Logger {
	if globalLogger == nil {
		// Initialize with default settings if not set
		globalLogger = NewLogger(LogLevelInfo, false)
	}
	return globalLogger
}

// InitGlobalLogger initializes the global logger with configuration
func InitGlobalLogger(level LogLevel, verbose bool, logFile string) error {
	logger := NewLogger(level, verbose)

	if logFile != "" {
		if err := logger.SetLogFile(logFile); err != nil {
			return err
		}
	}

	SetGlobalLogger(logger)
	return nil
}
