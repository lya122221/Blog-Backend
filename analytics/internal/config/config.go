package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port          int
	LogLevel      string
	LogFormat     string
	KafkaBrokers  []string
	KafkaTopic    string
	KafkaDLQTopic string
	KafkaGroup    string
	BatchSize     int
	FlushInterval time.Duration
	RetryAttempts int
	RetryBackoff  time.Duration
	CookieSecure  bool
	JWTPublicKey  string
	RedisAddr     string
	ClickHouse    ClickHouseConfig
}

func Load() (Config, error) {
	config := Config{
		LogLevel:  os.Getenv("LOG_LEVEL"),
		LogFormat: os.Getenv("LOG_FORMAT"),
	}
	brokers := os.Getenv("ANALYTICS_KAFKA_BROKERS")
	if strings.TrimSpace(brokers) == "" {
		return Config{}, fmt.Errorf("missing ANALYTICS_KAFKA_BROKERS")
	}
	for _, broker := range strings.Split(brokers, ",") {
		broker = strings.TrimSpace(broker)
		if broker == "" {
			return Config{}, fmt.Errorf("invalid ANALYTICS_KAFKA_BROKERS %q", brokers)
		}
		config.KafkaBrokers = append(config.KafkaBrokers, broker)
	}
	config.KafkaTopic = strings.TrimSpace(os.Getenv("ANALYTICS_KAFKA_TOPIC"))
	if config.KafkaTopic == "" {
		return Config{}, fmt.Errorf("missing ANALYTICS_KAFKA_TOPIC")
	}
	config.KafkaDLQTopic = strings.TrimSpace(os.Getenv("ANALYTICS_KAFKA_DLQ_TOPIC"))
	if config.KafkaDLQTopic == "" || config.KafkaDLQTopic == config.KafkaTopic {
		return Config{}, fmt.Errorf("invalid ANALYTICS_KAFKA_DLQ_TOPIC %q", config.KafkaDLQTopic)
	}
	config.KafkaGroup = strings.TrimSpace(os.Getenv("ANALYTICS_KAFKA_CONSUMER_GROUP"))
	if config.KafkaGroup == "" {
		return Config{}, fmt.Errorf("missing ANALYTICS_KAFKA_CONSUMER_GROUP")
	}
	batchSizeText := strings.TrimSpace(os.Getenv("ANALYTICS_KAFKA_BATCH_SIZE"))
	if batchSizeText == "" {
		return Config{}, fmt.Errorf("missing ANALYTICS_KAFKA_BATCH_SIZE")
	}
	batchSize, err := strconv.Atoi(batchSizeText)
	if err != nil || batchSize < 1 || batchSize > 10000 {
		return Config{}, fmt.Errorf("invalid ANALYTICS_KAFKA_BATCH_SIZE %q", batchSizeText)
	}
	config.BatchSize = batchSize
	flushText := strings.TrimSpace(os.Getenv("ANALYTICS_KAFKA_FLUSH_INTERVAL"))
	if flushText == "" {
		return Config{}, fmt.Errorf("missing ANALYTICS_KAFKA_FLUSH_INTERVAL")
	}
	flushInterval, err := time.ParseDuration(flushText)
	if err != nil || flushInterval <= 0 || flushInterval >= time.Minute {
		return Config{}, fmt.Errorf("invalid ANALYTICS_KAFKA_FLUSH_INTERVAL %q", flushText)
	}
	config.FlushInterval = flushInterval
	retryAttemptsText := strings.TrimSpace(os.Getenv("ANALYTICS_RETRY_ATTEMPTS"))
	retryAttempts, err := strconv.Atoi(retryAttemptsText)
	if err != nil || retryAttempts < 1 || retryAttempts > 5 {
		return Config{}, fmt.Errorf("invalid ANALYTICS_RETRY_ATTEMPTS %q", retryAttemptsText)
	}
	config.RetryAttempts = retryAttempts
	retryBackoffText := strings.TrimSpace(os.Getenv("ANALYTICS_RETRY_BACKOFF"))
	retryBackoff, err := time.ParseDuration(retryBackoffText)
	if err != nil || retryBackoff <= 0 || retryBackoff > time.Second {
		return Config{}, fmt.Errorf("invalid ANALYTICS_RETRY_BACKOFF %q", retryBackoffText)
	}
	config.RetryBackoff = retryBackoff
	config.RedisAddr = strings.TrimSpace(os.Getenv("ANALYTICS_REDIS_ADDR"))
	if config.RedisAddr == "" {
		return Config{}, fmt.Errorf("missing ANALYTICS_REDIS_ADDR")
	}
	config.JWTPublicKey = strings.TrimSpace(os.Getenv("ANALYTICS_JWT_PUBLIC_KEY"))
	if config.JWTPublicKey == "" {
		return Config{}, fmt.Errorf("missing ANALYTICS_JWT_PUBLIC_KEY")
	}
	if value := os.Getenv("ANALYTICS_COOKIE_SECURE"); value != "" {
		cookieSecure, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("invalid ANALYTICS_COOKIE_SECURE %q", value)
		}
		config.CookieSecure = cookieSecure
	}

	portText := strings.TrimSpace(os.Getenv("ANALYTICS_PORT"))
	if portText == "" {
		return Config{}, fmt.Errorf("missing ANALYTICS_PORT")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("invalid ANALYTICS_PORT %q", portText)
	}
	config.Port = port

	clickHouse, err := LoadClickHouse()
	if err != nil {
		return Config{}, err
	}
	config.ClickHouse = clickHouse

	return config, nil
}

func (c Config) Address() string {
	return ":" + strconv.Itoa(c.Port)
}
