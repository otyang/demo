package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestNewJSONLoggerOptions(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(Options{
		Format:    FormatJSON,
		Level:     slog.LevelDebug,
		AddSource: true,
		Output:    &output,
		AddPID:    true,
		Version:   "1.2.3",
		Build:     "abc123",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Info("server started", "port", 8080)

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode JSON log: %v", err)
	}
	if record["msg"] != "server started" {
		t.Errorf("msg = %v, want %q", record["msg"], "server started")
	}
	if record["version"] != "1.2.3" {
		t.Errorf("version = %v, want %q", record["version"], "1.2.3")
	}
	if record["build"] != "abc123" {
		t.Errorf("build = %v, want %q", record["build"], "abc123")
	}
	if pid, ok := record["pid"].(float64); !ok || int(pid) != os.Getpid() {
		t.Errorf("pid = %v, want %d", record["pid"], os.Getpid())
	}
	if record["source"] == nil {
		t.Error("source field is missing")
	}
	if record["port"] != float64(8080) {
		t.Errorf("port = %v, want 8080", record["port"])
	}
}

func TestNewTextLoggerAndLevelFilter(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(Options{
		Format: FormatText,
		Level:  slog.LevelWarn,
		Output: &output,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Info("filtered message")
	logger.Warn("visible message")
	if strings.Contains(output.String(), "filtered message") {
		t.Error("info message was emitted below configured warn level")
	}
	if !strings.Contains(output.String(), "visible message") {
		t.Error("warn message was not emitted")
	}
}

func TestNewDefaultsToTextAndInfo(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(Options{Output: &output})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Debug("hidden")
	logger.Info("visible")
	if strings.Contains(output.String(), "hidden") {
		t.Error("debug message was emitted at default info level")
	}
	if !strings.Contains(output.String(), "visible") {
		t.Error("info message was not emitted")
	}
	if !strings.Contains(output.String(), "level=INFO") {
		t.Errorf("output %q is not text format", output.String())
	}
}

func TestNewRejectsUnsupportedFormat(t *testing.T) {
	if _, err := New(Options{Format: "xml"}); err == nil {
		t.Fatal("New() error = nil, want unsupported format error")
	}
}

func TestNewDevelopmentPreset(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewDevelopment(Options{Output: &output})
	if err != nil {
		t.Fatalf("NewDevelopment() error = %v", err)
	}

	logger.Debug("development detail", "user_id", 42)
	got := output.String()
	if !strings.Contains(got, "DBG") || !strings.Contains(got, "development detail") || !strings.Contains(got, "user_id=42") {
		t.Errorf("development logger output = %q, want a debug color text log", got)
	}
	if !strings.Contains(got, "logger_test.go:") {
		t.Errorf("development logger output = %q, want the source location", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Errorf("development logger output = %q, want no color codes when output is not a terminal", got)
	}
}

func TestColorFormatRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if colorSupported(os.Stderr) {
		t.Error("colorSupported() = true with NO_COLOR set")
	}
	t.Setenv("NO_COLOR", "")
	var output bytes.Buffer
	if colorSupported(&output) {
		t.Error("colorSupported() = true for a writer that is not a terminal")
	}
}

func TestNewProductionPreset(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewProduction(Options{Output: &output})
	if err != nil {
		t.Fatalf("NewProduction() error = %v", err)
	}

	logger.Debug("filtered detail")
	logger.Info("production event")
	if strings.Contains(output.String(), "filtered detail") {
		t.Error("production logger emitted debug message")
	}
	if !strings.Contains(output.String(), `"msg":"production event"`) {
		t.Errorf("production logger output = %q, want JSON info log", output.String())
	}
	if strings.Contains(output.String(), `"source"`) {
		t.Errorf("production logger output = %q, did not want source field", output.String())
	}
}

func TestSetDefaultEnablesSlogShortcuts(t *testing.T) {
	previous := slog.Default()
	defer slog.SetDefault(previous)

	var output bytes.Buffer
	logger, err := NewDevelopment(Options{Output: &output})
	if err != nil {
		t.Fatalf("NewDevelopment() error = %v", err)
	}
	if err := SetDefault(logger); err != nil {
		t.Fatalf("SetDefault() error = %v", err)
	}

	slog.Debug("package debug")
	slog.Info("package info")
	if !strings.Contains(output.String(), "package debug") {
		t.Error("slog.Debug did not write to configured default logger")
	}
	if !strings.Contains(output.String(), "package info") {
		t.Error("slog.Info did not write to configured default logger")
	}
}

func TestSetDefaultRejectsNil(t *testing.T) {
	if err := SetDefault(nil); err == nil {
		t.Fatal("SetDefault(nil) error = nil, want error")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{" error ", slog.LevelError},
		{"", slog.LevelInfo},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := ParseLevel(test.input)
			if err != nil {
				t.Fatalf("ParseLevel() error = %v", err)
			}
			if got != test.want {
				t.Errorf("ParseLevel() = %v, want %v", got, test.want)
			}
		})
	}

	if _, err := ParseLevel("verbose"); err == nil {
		t.Error("ParseLevel() error = nil, want invalid-level error")
	}
}
