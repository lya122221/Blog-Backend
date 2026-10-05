package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Outbox struct {
	KafkaBrokers    []string
	KafkaTopic      string
	BatchSize       int
	PollInterval    time.Duration
	RetryInterval   time.Duration
	CleanupInterval time.Duration
	Retention       time.Duration
}

func LoadOutbox() (Outbox, error) {
	var cfg Outbox
	for _, broker := range strings.Split(os.Getenv("BLOG_KAFKA_BROKERS"), ",") {
		broker = strings.TrimSpace(broker)
		if broker != "" {
			cfg.KafkaBrokers = append(cfg.KafkaBrokers, broker)
		}
	}
	if len(cfg.KafkaBrokers) == 0 {
		return cfg, fmt.Errorf("BLOG_KAFKA_BROKERS is required")
	}
	cfg.KafkaTopic = strings.TrimSpace(os.Getenv("BLOG_KAFKA_TOPIC"))
	if cfg.KafkaTopic == "" {
		return cfg, fmt.Errorf("BLOG_KAFKA_TOPIC is required")
	}
	var err error
	cfg.BatchSize, err = positiveInt("BLOG_OUTBOX_BATCH_SIZE", 100)
	if err != nil {
		return cfg, err
	}
	for _, setting := range []struct {
		name  string
		value *time.Duration
		def   time.Duration
	}{
		{"BLOG_OUTBOX_POLL_INTERVAL", &cfg.PollInterval, time.Second},
		{"BLOG_OUTBOX_RETRY_INTERVAL", &cfg.RetryInterval, 2 * time.Second},
		{"BLOG_OUTBOX_CLEANUP_INTERVAL", &cfg.CleanupInterval, time.Hour},
		{"BLOG_OUTBOX_RETENTION", &cfg.Retention, 7 * 24 * time.Hour},
	} {
		*setting.value, err = positiveDuration(setting.name, setting.def)
		if err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func positiveInt(name string, def int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return def, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

func positiveDuration(name string, def time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return def, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}
