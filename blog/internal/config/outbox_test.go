package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadOutbox(t *testing.T) {
	t.Setenv("BLOG_KAFKA_BROKERS", "kafka-1:9092, kafka-2:9092")
	t.Setenv("BLOG_KAFKA_TOPIC", "article-events")
	cfg, err := LoadOutbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.KafkaBrokers) != 2 || cfg.KafkaBrokers[1] != "kafka-2:9092" || cfg.KafkaTopic != "article-events" || cfg.BatchSize != 100 || cfg.PollInterval != time.Second || cfg.Retention != 7*24*time.Hour {
		t.Fatalf("unexpected outbox config: %+v", cfg)
	}
	t.Setenv("BLOG_OUTBOX_BATCH_SIZE", "20")
	t.Setenv("BLOG_OUTBOX_POLL_INTERVAL", "250ms")
	cfg, err = LoadOutbox()
	if err != nil || cfg.BatchSize != 20 || cfg.PollInterval != 250*time.Millisecond {
		t.Fatalf("custom outbox config = %+v, %v", cfg, err)
	}
}

func TestLoadOutboxRejectsInvalidSettings(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"BLOG_KAFKA_BROKERS", ""},
		{"BLOG_KAFKA_TOPIC", ""},
		{"BLOG_OUTBOX_BATCH_SIZE", "0"},
		{"BLOG_OUTBOX_POLL_INTERVAL", "bad"},
		{"BLOG_OUTBOX_RETRY_INTERVAL", "0s"},
		{"BLOG_OUTBOX_CLEANUP_INTERVAL", "-1s"},
		{"BLOG_OUTBOX_RETENTION", "bad"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("BLOG_KAFKA_BROKERS", "kafka:9092")
			t.Setenv("BLOG_KAFKA_TOPIC", "article-events")
			for _, name := range []string{
				"BLOG_OUTBOX_BATCH_SIZE", "BLOG_OUTBOX_POLL_INTERVAL", "BLOG_OUTBOX_RETRY_INTERVAL", "BLOG_OUTBOX_CLEANUP_INTERVAL", "BLOG_OUTBOX_RETENTION",
			} {
				t.Setenv(name, "")
			}
			t.Setenv(test.name, test.value)
			if _, err := LoadOutbox(); err == nil || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("expected %s error, got %v", test.name, err)
			}
		})
	}
}
