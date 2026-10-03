package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port         int
	LogLevel     string
	LogFormat    string
	KafkaBrokers []string
	KafkaTopic   string
	CookieSecure bool
	ClickHouse   ClickHouseConfig
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
