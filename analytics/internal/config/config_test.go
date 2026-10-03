package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ANALYTICS_PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("ANALYTICS_KAFKA_BROKERS", "")
	t.Setenv("ANALYTICS_KAFKA_TOPIC", "")
	t.Setenv("ANALYTICS_COOKIE_SECURE", "")

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Port != 8082 || settings.Address() != ":8082" {
		t.Fatalf("unexpected default address: %+v", settings)
	}
	if len(settings.KafkaBrokers) != 1 || settings.KafkaBrokers[0] != "localhost:9092" || settings.KafkaTopic != "article-events" || settings.CookieSecure {
		t.Fatalf("unexpected Kafka/cookie defaults: %+v", settings)
	}
}

func TestLoadCustomSettings(t *testing.T) {
	t.Setenv("ANALYTICS_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("ANALYTICS_KAFKA_BROKERS", "kafka-1:9092, kafka-2:9092")
	t.Setenv("ANALYTICS_KAFKA_TOPIC", "views")
	t.Setenv("ANALYTICS_COOKIE_SECURE", "true")

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Address() != ":9090" || settings.LogLevel != "debug" || settings.LogFormat != "text" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
	if len(settings.KafkaBrokers) != 2 || settings.KafkaBrokers[0] != "kafka-1:9092" || settings.KafkaBrokers[1] != "kafka-2:9092" || settings.KafkaTopic != "views" || !settings.CookieSecure {
		t.Fatalf("unexpected Kafka/cookie settings: %+v", settings)
	}
}

func TestLoadRejectsInvalidKafkaAndCookieSettings(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"empty broker":        {"ANALYTICS_KAFKA_BROKERS": "kafka:9092,"},
		"empty topic":         {"ANALYTICS_KAFKA_TOPIC": " "},
		"invalid secure flag": {"ANALYTICS_COOKIE_SECURE": "maybe"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ANALYTICS_PORT", "")
			t.Setenv("ANALYTICS_KAFKA_BROKERS", "")
			t.Setenv("ANALYTICS_KAFKA_TOPIC", "")
			t.Setenv("ANALYTICS_COOKIE_SECURE", "")
			for key, value := range env {
				t.Setenv(key, value)
			}
			if _, err := Load(); err == nil {
				t.Fatal("Load accepted invalid settings")
			}
		})
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
