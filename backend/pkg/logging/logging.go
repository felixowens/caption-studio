// Package logging provides a logging system for the application.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// LogLevel is the level of the log.
type LogLevel string

// LogFormat is the format of the log.
type LogFormat string

const (
	// LogLevelDebug is the debug level. This is the most verbose level.
	LogLevelDebug LogLevel = "debug"
	// LogLevelInfo is the info level. This is the default level.
	LogLevelInfo LogLevel = "info"
	// LogLevelWarn is the warn level. This is a warning level.
	LogLevelWarn LogLevel = "warn"
	// LogLevelError is the error level. This is an error level.
	LogLevelError LogLevel = "error"
)

// Config is the configuration for the logging system.
type Config struct {
	Level  LogLevel
	Format LogFormat
}

// Setup sets up the logging system.
func Setup(cfg Config) *slog.Logger {
	level := parseLevel(cfg.Level)

	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug,
	}

	switch strings.ToLower(string(cfg.Format)) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

func parseLevel(level LogLevel) slog.Level {
	switch strings.ToLower(string(level)) {
	case string(LogLevelDebug):
		return slog.LevelDebug
	case string(LogLevelInfo):
		return slog.LevelInfo
	case string(LogLevelWarn):
		return slog.LevelWarn
	case string(LogLevelError):
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithContext adds context to the logger.
func WithContext(_ context.Context, logger *slog.Logger, attrs ...slog.Attr) *slog.Logger {
	return logger.With(slog.Group("context", attrsToAny(attrs)...))
}

func attrsToAny(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for i, attr := range attrs {
		result[i] = attr
	}
	return result
}

// TODO: add child loggers and correlation IDs
