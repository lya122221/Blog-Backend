package config

import (
	"strings"
	"testing"
)

func TestLoadClickHouseRequiredSettings(t *testing.T) {
	setRequiredSettings(t)
	t.Setenv("ANALYTICS_CLICKHOUSE_PASSWORD", "")

	settings, err := LoadClickHouse()
	if err != nil {
		t.Fatalf("LoadClickHouse: %v", err)
	}
	if settings.Addr != "localhost:9000" || settings.Database != "default" || settings.User != "default" || settings.Password != "" {
		t.Fatalf("unexpected ClickHouse settings: %+v", settings)
	}
}

func TestLoadClickHouseCustomSettings(t *testing.T) {
	setRequiredSettings(t)
	t.Setenv("ANALYTICS_CLICKHOUSE_ADDR", " clickhouse:9000 ")
	t.Setenv("ANALYTICS_CLICKHOUSE_DATABASE", " analytics ")
	t.Setenv("ANALYTICS_CLICKHOUSE_USER", " reader ")
	t.Setenv("ANALYTICS_CLICKHOUSE_PASSWORD", " secret with spaces ")

	settings, err := LoadClickHouse()
	if err != nil {
		t.Fatalf("LoadClickHouse: %v", err)
	}
	if settings.Addr != "clickhouse:9000" || settings.Database != "analytics" || settings.User != "reader" || settings.Password != " secret with spaces " {
		t.Fatalf("unexpected ClickHouse settings: %+v", settings)
	}
}

func TestLoadClickHouseRejectsInvalidSettings(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"missing host":   {"ANALYTICS_CLICKHOUSE_ADDR": ":9000"},
		"missing port":   {"ANALYTICS_CLICKHOUSE_ADDR": "clickhouse"},
		"invalid port":   {"ANALYTICS_CLICKHOUSE_ADDR": "clickhouse:abc"},
		"port too high":  {"ANALYTICS_CLICKHOUSE_ADDR": "clickhouse:65536"},
		"empty address":  {"ANALYTICS_CLICKHOUSE_ADDR": " "},
		"empty database": {"ANALYTICS_CLICKHOUSE_DATABASE": " "},
		"empty user":     {"ANALYTICS_CLICKHOUSE_USER": " "},
	} {
		t.Run(name, func(t *testing.T) {
			setRequiredSettings(t)
			for key, value := range env {
				t.Setenv(key, value)
			}
			if _, err := LoadClickHouse(); err == nil || !strings.Contains(err.Error(), "ANALYTICS_CLICKHOUSE_") {
				t.Fatalf("LoadClickHouse error = %v", err)
			}
		})
	}
}

func TestLoadClickHouseRejectsMissingSettings(t *testing.T) {
	for _, name := range []string{"ANALYTICS_CLICKHOUSE_ADDR", "ANALYTICS_CLICKHOUSE_DATABASE", "ANALYTICS_CLICKHOUSE_USER"} {
		t.Run(name, func(t *testing.T) {
			setRequiredSettings(t)
			t.Setenv(name, "")
			if _, err := LoadClickHouse(); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("LoadClickHouse error = %v", err)
			}
		})
	}
}
