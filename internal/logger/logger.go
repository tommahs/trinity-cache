// Package logger provides structured logging for Trinity-cache using Go's standard log/slog.
// Following Go philosophy: simple, explicit, and using the standard library.
package logger

import (
	"log/slog"
	"os"
)

// Logger wraps slog.Logger for convenient access.
var log *slog.Logger

func init() {
	// Initialize with default configuration: info level to stdout
	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
}

// SetLevel reconfigures the logger's level. Call this once during application startup
// after configuration is loaded.
func SetLevel(level slog.Level) {
	opts := &slog.HandlerOptions{Level: level}
	log = slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(log)
}

// ParseLevel converts a string to slog.Level. Returns info level for unrecognized values.
func ParseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default: // "info" or unrecognized
		return slog.LevelInfo
	}
}

// Debug logs a debug-level message. Use sparingly for detailed diagnostic information.
func Debug(msg string, args ...any) {
	log.Debug(msg, args...)
}

// Info logs an info-level message. Use for notable application events.
func Info(msg string, args ...any) {
	log.Info(msg, args...)
}

// Warn logs a warning-level message. Use for recoverable issues.
func Warn(msg string, args ...any) {
	log.Warn(msg, args...)
}

// Error logs an error-level message. Use when operations fail.
func Error(msg string, args ...any) {
	log.Error(msg, args...)
}

// With returns a logger with pre-set fields. Useful for adding context to multiple log calls.
func With(args ...any) *slog.Logger {
	return log.With(args...)
}
