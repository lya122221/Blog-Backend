package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewJSONLoggerHonorsLevel(t *testing.T) {
	var output bytes.Buffer
	log, err := New(&output, Config{Level: "warn", Format: "json"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	log.Info("hidden")
	log.Warn("visible", "component", "test")

	if strings.Contains(output.String(), "hidden") {
		t.Fatalf("info message was not filtered: %s", output.String())
	}
	if !strings.Contains(output.String(), `"level":"WARN"`) ||
		!strings.Contains(output.String(), `"component":"test"`) {
		t.Fatalf("unexpected JSON log: %s", output.String())
	}
}

func TestNewTextLogger(t *testing.T) {
	var output bytes.Buffer
	log, err := New(&output, Config{Level: "debug", Format: "text"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	log.Debug("debug message")
	if !strings.Contains(output.String(), "level=DEBUG") {
		t.Fatalf("unexpected text log: %s", output.String())
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	tests := []Config{
		{Level: "verbose", Format: "json"},
		{Level: "info", Format: "xml"},
	}

	for _, config := range tests {
		if _, err := New(&bytes.Buffer{}, config); err == nil {
			t.Fatalf("expected error for config %+v", config)
		}
	}
}

func TestNewUsesDefaults(t *testing.T) {
	var output bytes.Buffer
	log, err := New(&output, Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Info("default logger")
	if !strings.Contains(output.String(), `"msg":"default logger"`) {
		t.Fatalf("unexpected default log: %s", output.String())
	}
}
