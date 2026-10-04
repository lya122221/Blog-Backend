package config

import (
	"strings"
	"testing"
	"time"
)

func setRequiredSettings(t *testing.T) {
	t.Helper()
	t.Setenv("ANALYTICS_PORT", "8082")
	t.Setenv("ANALYTICS_KAFKA_BROKERS", "localhost:9092")
	t.Setenv("ANALYTICS_KAFKA_TOPIC", "article-events")
	t.Setenv("ANALYTICS_KAFKA_DLQ_TOPIC", "article-events.dlq")
	t.Setenv("ANALYTICS_KAFKA_CONSUMER_GROUP", "analytics-events")
	t.Setenv("ANALYTICS_KAFKA_BATCH_SIZE", "500")
	t.Setenv("ANALYTICS_KAFKA_FLUSH_INTERVAL", "1s")
	t.Setenv("ANALYTICS_RETRY_ATTEMPTS", "3")
	t.Setenv("ANALYTICS_RETRY_BACKOFF", "100ms")
	t.Setenv("ANALYTICS_CLICKHOUSE_ADDR", "localhost:9000")
	t.Setenv("ANALYTICS_CLICKHOUSE_DATABASE", "default")
	t.Setenv("ANALYTICS_CLICKHOUSE_USER", "default")
}

func TestLoadRequiredSettingsAndOptionalDefaults(t *testing.T) {
	setRequiredSettings(t)
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("ANALYTICS_COOKIE_SECURE", "")
	t.Setenv("ANALYTICS_CLICKHOUSE_PASSWORD", "")

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Port != 8082 || settings.Address() != ":8082" {
		t.Fatalf("unexpected address: %+v", settings)
	}
	if len(settings.KafkaBrokers) != 1 || settings.KafkaBrokers[0] != "localhost:9092" || settings.KafkaTopic != "article-events" || settings.CookieSecure {
		t.Fatalf("unexpected Kafka/cookie settings: %+v", settings)
	}
	if settings.KafkaDLQTopic != "article-events.dlq" || settings.KafkaGroup != "analytics-events" || settings.BatchSize != 500 || settings.FlushInterval != time.Second || settings.RetryAttempts != 3 || settings.RetryBackoff != 100*time.Millisecond {
		t.Fatalf("unexpected consumer settings: %+v", settings)
	}
	if settings.ClickHouse.Addr != "localhost:9000" || settings.ClickHouse.Database != "default" || settings.ClickHouse.User != "default" {
		t.Fatalf("unexpected ClickHouse settings: %+v", settings.ClickHouse)
	}
}

func TestLoadCustomSettings(t *testing.T) {
	setRequiredSettings(t)
	t.Setenv("ANALYTICS_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("ANALYTICS_KAFKA_BROKERS", "kafka-1:9092, kafka-2:9092")
	t.Setenv("ANALYTICS_KAFKA_TOPIC", "views")
	t.Setenv("ANALYTICS_KAFKA_DLQ_TOPIC", "views.dlq")
	t.Setenv("ANALYTICS_KAFKA_CONSUMER_GROUP", "custom-consumer")
	t.Setenv("ANALYTICS_KAFKA_BATCH_SIZE", "100")
	t.Setenv("ANALYTICS_KAFKA_FLUSH_INTERVAL", "250ms")
	t.Setenv("ANALYTICS_RETRY_ATTEMPTS", "5")
	t.Setenv("ANALYTICS_RETRY_BACKOFF", "25ms")
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
	if settings.KafkaDLQTopic != "views.dlq" || settings.KafkaGroup != "custom-consumer" || settings.BatchSize != 100 || settings.FlushInterval != 250*time.Millisecond || settings.RetryAttempts != 5 || settings.RetryBackoff != 25*time.Millisecond {
		t.Fatalf("unexpected consumer settings: %+v", settings)
	}
}

func TestLoadRejectsInvalidConsumerSettings(t *testing.T) {
	for name, test := range map[string]struct {
		key   string
		value string
	}{
		"missing group":    {"ANALYTICS_KAFKA_CONSUMER_GROUP", ""},
		"missing size":     {"ANALYTICS_KAFKA_BATCH_SIZE", ""},
		"zero size":        {"ANALYTICS_KAFKA_BATCH_SIZE", "0"},
		"size too large":   {"ANALYTICS_KAFKA_BATCH_SIZE", "10001"},
		"non-numeric size": {"ANALYTICS_KAFKA_BATCH_SIZE", "many"},
		"missing interval": {"ANALYTICS_KAFKA_FLUSH_INTERVAL", ""},
		"zero interval":    {"ANALYTICS_KAFKA_FLUSH_INTERVAL", "0s"},
		"long interval":    {"ANALYTICS_KAFKA_FLUSH_INTERVAL", "1m"},
		"invalid interval": {"ANALYTICS_KAFKA_FLUSH_INTERVAL", "fast"},
		"missing attempts": {"ANALYTICS_RETRY_ATTEMPTS", ""},
		"zero attempts":    {"ANALYTICS_RETRY_ATTEMPTS", "0"},
		"many attempts":    {"ANALYTICS_RETRY_ATTEMPTS", "6"},
		"missing backoff":  {"ANALYTICS_RETRY_BACKOFF", ""},
		"zero backoff":     {"ANALYTICS_RETRY_BACKOFF", "0s"},
		"long backoff":     {"ANALYTICS_RETRY_BACKOFF", "2s"},
	} {
		t.Run(name, func(t *testing.T) {
			setRequiredSettings(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load error = %v", err)
			}
		})
	}
}

func TestLoadRejectsInvalidKafkaAndCookieSettings(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"missing broker":      {"ANALYTICS_KAFKA_BROKERS": ""},
		"empty broker":        {"ANALYTICS_KAFKA_BROKERS": "kafka:9092,"},
		"missing topic":       {"ANALYTICS_KAFKA_TOPIC": ""},
		"empty topic":         {"ANALYTICS_KAFKA_TOPIC": " "},
		"missing DLQ topic":   {"ANALYTICS_KAFKA_DLQ_TOPIC": ""},
		"same DLQ topic":      {"ANALYTICS_KAFKA_DLQ_TOPIC": "article-events"},
		"invalid secure flag": {"ANALYTICS_COOKIE_SECURE": "maybe"},
	} {
		t.Run(name, func(t *testing.T) {
			setRequiredSettings(t)
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
	for _, value := range []string{"", " ", "abc", "0", "-1", "65536"} {
		t.Run(value, func(t *testing.T) {
			setRequiredSettings(t)
			t.Setenv("ANALYTICS_PORT", value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ANALYTICS_PORT") {
				t.Fatalf("Load error = %v", err)
			}
		})
	}
}
