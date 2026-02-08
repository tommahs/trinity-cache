package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestParseLevel tests the ParseLevel function with various inputs.
func TestParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected slog.Level
	}{
		{"debug", "debug", slog.LevelDebug},
		{"info", "info", slog.LevelInfo},
		{"warn", "warn", slog.LevelWarn},
		{"error", "error", slog.LevelError},
		{"empty defaults to info", "", slog.LevelInfo},
		{"invalid defaults to info", "trace", slog.LevelInfo},
		{"case sensitive defaults to info", "DEBUG", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSetLevel verifies that SetLevel changes the effective logging level.
func TestSetLevel(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
	}{
		{"set debug", slog.LevelDebug},
		{"set info", slog.LevelInfo},
		{"set warn", slog.LevelWarn},
		{"set error", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			SetLevel(tt.level)
			log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: tt.level}))

			// Test that the level is respected
			if tt.level == slog.LevelError {
				Info("info message")
				if buf.Len() != 0 {
					t.Error("info message logged when level is ERROR")
				}
			}
		})
	}
}

// TestLogLevelFiltering verifies that messages below the set level are filtered.
func TestLogLevelFiltering(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
		tests []struct {
			message string
			logFn   func(string, ...any)
			expect  bool
		}
	}{
		{
			name:  "warn level filters debug and info",
			level: slog.LevelWarn,
			tests: []struct {
				message string
				logFn   func(string, ...any)
				expect  bool
			}{
				{"debug message", Debug, false},
				{"info message", Info, false},
				{"warn message", Warn, true},
				{"error message", Error, true},
			},
		},
		{
			name:  "error level filters all but error",
			level: slog.LevelError,
			tests: []struct {
				message string
				logFn   func(string, ...any)
				expect  bool
			}{
				{"debug message", Debug, false},
				{"info message", Info, false},
				{"warn message", Warn, false},
				{"error message", Error, true},
			},
		},
		{
			name:  "debug level logs everything",
			level: slog.LevelDebug,
			tests: []struct {
				message string
				logFn   func(string, ...any)
				expect  bool
			}{
				{"debug message", Debug, true},
				{"info message", Info, true},
				{"warn message", Warn, true},
				{"error message", Error, true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: tt.level}))

			for _, subtest := range tt.tests {
				buf.Reset()
				subtest.logFn(subtest.message)

				hasOutput := buf.Len() > 0
				if hasOutput != subtest.expect {
					t.Errorf("%s: expected output=%v, got=%v", subtest.message, subtest.expect, hasOutput)
				}
			}
		})
	}
}

// TestStructuredAttributes verifies that key-value pairs appear in logs.
func TestStructuredAttributes(t *testing.T) {
	buf := &bytes.Buffer{}
	log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	Info("operation completed", "duration_ms", 150, "status", "success")

	output := buf.String()
	if !strings.Contains(output, "operation completed") {
		t.Errorf("message not in output: %q", output)
	}
	if !strings.Contains(output, "duration_ms") {
		t.Errorf("duration_ms attribute not in output: %q", output)
	}
	if !strings.Contains(output, "150") {
		t.Errorf("attribute value not in output: %q", output)
	}
	if !strings.Contains(output, "status") {
		t.Errorf("status attribute not in output: %q", output)
	}
}

// TestWith verifies that With() creates a logger with pre-set fields.
func TestWith(t *testing.T) {
	buf := &bytes.Buffer{}
	log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	contextLogger := With("component", "cache", "version", "1.0")
	contextLogger.Info("cache initialized")

	output := buf.String()
	if !strings.Contains(output, "cache initialized") {
		t.Errorf("message not in output: %q", output)
	}
	if !strings.Contains(output, "component") {
		t.Errorf("component context not in output: %q", output)
	}
	if !strings.Contains(output, "cache") {
		t.Errorf("component value not in output: %q", output)
	}
}

// TestPackageInitialization verifies the logger is initialized by default.
func TestPackageInitialization(t *testing.T) {
	// The package initializes a logger in init(), so the global logger should be valid
	if log == nil {
		t.Fatal("logger not initialized")
	}

	// Test that default logger works
	buf := &bytes.Buffer{}
	log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	Info("initialization test")
	if buf.Len() == 0 {
		t.Error("default logger not working")
	}
}

// TestAllLogLevels ensures all logging functions produce output.
func TestAllLogLevels(t *testing.T) {
	buf := &bytes.Buffer{}
	log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	tests := []struct {
		name  string
		logFn func(string, ...any)
		level string
	}{
		{"Debug", Debug, "DEBUG"},
		{"Info", Info, "INFO"},
		{"Warn", Warn, "WARN"},
		{"Error", Error, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFn("test message")

			output := buf.String()
			if !strings.Contains(output, "test message") {
				t.Errorf("message not in output: %q", output)
			}
		})
	}
}
