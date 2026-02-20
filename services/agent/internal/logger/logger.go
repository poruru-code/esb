// Package logger provides a centralized slog-based logger with level and format control.
package logger

import (
	"log/slog"
	"os"
	"strings"

	"github.com/poruru-code/esb/services/agent/internal/config"
)

var defaultLogger *slog.Logger

// Init initializes the global logger based on environment variables.
// Priority: AGENT_LOG_LEVEL > LOG_LEVEL > Default ("info")
// AGENT_LOG_FORMAT: text, json (default: text)
func Init() {
	levelStr := os.Getenv("AGENT_LOG_LEVEL")
	if levelStr == "" {
		levelStr = os.Getenv("LOG_LEVEL")
	}
	level := parseLevel(levelStr)
	formatStr := os.Getenv("AGENT_LOG_FORMAT")
	if formatStr == "" {
		formatStr = config.DefaultLogFormat
	}
	format := strings.ToLower(formatStr)

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		if s == "" {
			return parseLevel(config.DefaultLogLevel)
		}
		return slog.LevelInfo
	}
}

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// Info logs at info level.
func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// Error logs at error level.
func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}
