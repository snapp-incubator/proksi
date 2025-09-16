package logging

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// L is the default logger of the application
	L *zap.Logger
)

func init() {
	// Initialize with default production logger
	// This will be replaced when InitializeLogger is called
	L, _ = zap.NewProduction(zap.WithCaller(false))
}

// InitializeLogger configures the logger with the specified log level
func InitializeLogger(logLevel string) error {
	level, err := parseLogLevel(logLevel)
	if err != nil {
		return fmt.Errorf("invalid log level '%s': %w", logLevel, err)
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	config.DisableCaller = true

	logger, err := config.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger: %w", err)
	}

	L = logger
	return nil
}

// parseLogLevel converts string log level to zapcore.Level
func parseLogLevel(logLevel string) (zapcore.Level, error) {
	switch strings.ToLower(logLevel) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("supported levels are: debug, info, warn, error, fatal")
	}
}
