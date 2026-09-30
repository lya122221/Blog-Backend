package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ANALYTICS_PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Port != 8082 || settings.Address() != ":8082" {
		t.Fatalf("unexpected default address: %+v", settings)
	}
}

func TestLoadCustomSettings(t *testing.T) {
	t.Setenv("ANALYTICS_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Address() != ":9090" || settings.LogLevel != "debug" || settings.LogFormat != "text" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, value := range []string{"abc", "0", "-1", "65536"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("ANALYTICS_PORT", value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ANALYTICS_PORT") {
				t.Fatalf("Load error = %v", err)
			}
		})
	}
}
