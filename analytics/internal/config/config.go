package config

import (
	"fmt"
	"os"
	"strconv"
)

const defaultPort = 8082

type Config struct {
	Port      int
	LogLevel  string
	LogFormat string
}

func Load() (Config, error) {
	config := Config{
		Port:      defaultPort,
		LogLevel:  os.Getenv("LOG_LEVEL"),
		LogFormat: os.Getenv("LOG_FORMAT"),
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
