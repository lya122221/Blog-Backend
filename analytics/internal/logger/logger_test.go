package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewFiltersByLevel(t *testing.T) {
	var output bytes.Buffer
	log, err := New(&output, Config{Level: "warn", Format: "json"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Info("hidden")
	log.Warn("visible", "component", "analytics")
	if strings.Contains(output.String(), "hidden") || !strings.Contains(output.String(), `"component":"analytics"`) {
		t.Fatalf("unexpected log: %s", output.String())
	}
}

func TestNewTextLogger(t *testing.T) {
	var output bytes.Buffer
	log, err := New(&output, Config{Level: "debug", Format: "text"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Debug("message")
	if !strings.Contains(output.String(), "level=DEBUG") {
		t.Fatalf("unexpected log: %s", output.String())
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	for _, setting := range []Config{
		{Level: "verbose", Format: "json"},
		{Level: "info", Format: "xml"},
	} {
		if _, err := New(&bytes.Buffer{}, setting); err == nil {
			t.Fatalf("expected error for %+v", setting)
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
		t.Fatalf("unexpected log: %s", output.String())
	}
}
