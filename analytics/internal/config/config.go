package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultPort       = 8082
	defaultKafkaTopic = "article-events"
)

type Config struct {
	Port         int
	LogLevel     string
	LogFormat    string
	KafkaBrokers []string
	KafkaTopic   string
	CookieSecure bool
}

func Load() (Config, error) {
	config := Config{
		Port:         defaultPort,
		LogLevel:     os.Getenv("LOG_LEVEL"),
		LogFormat:    os.Getenv("LOG_FORMAT"),
		KafkaBrokers: []string{"localhost:9092"},
		KafkaTopic:   defaultKafkaTopic,
	}
	if value := os.Getenv("ANALYTICS_KAFKA_BROKERS"); value != "" {
		config.KafkaBrokers = nil
		for _, broker := range strings.Split(value, ",") {
			broker = strings.TrimSpace(broker)
			if broker == "" {
				return Config{}, fmt.Errorf("invalid ANALYTICS_KAFKA_BROKERS %q", value)
			}
			config.KafkaBrokers = append(config.KafkaBrokers, broker)
		}
	}
	if value := os.Getenv("ANALYTICS_KAFKA_TOPIC"); value != "" {
		config.KafkaTopic = strings.TrimSpace(value)
		if config.KafkaTopic == "" {
			return Config{}, fmt.Errorf("invalid ANALYTICS_KAFKA_TOPIC %q", value)
		}
	}
	if value := os.Getenv("ANALYTICS_COOKIE_SECURE"); value != "" {
		cookieSecure, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("invalid ANALYTICS_COOKIE_SECURE %q", value)
		}
		config.CookieSecure = cookieSecure
	}

	if value := os.Getenv("ANALYTICS_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("invalid ANALYTICS_PORT %q", value)
		}
		config.Port = port
	}

	return config, nil
}

func (c Config) Address() string {
	return ":" + strconv.Itoa(c.Port)
}
