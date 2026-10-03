package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type ClickHouseConfig struct {
	Addr     string
	Database string
	User     string
	Password string
}

func LoadClickHouse() (ClickHouseConfig, error) {
	config := ClickHouseConfig{
		Password: os.Getenv("ANALYTICS_CLICKHOUSE_PASSWORD"),
	}
	for _, setting := range []struct {
		name   string
		target *string
	}{
		{"ANALYTICS_CLICKHOUSE_ADDR", &config.Addr},
		{"ANALYTICS_CLICKHOUSE_DATABASE", &config.Database},
		{"ANALYTICS_CLICKHOUSE_USER", &config.User},
	} {
		value := strings.TrimSpace(os.Getenv(setting.name))
		if value == "" {
			return ClickHouseConfig{}, fmt.Errorf("missing %s", setting.name)
		}
		*setting.target = value
	}
	addr, portText, err := net.SplitHostPort(config.Addr)
	if err != nil || addr == "" {
		return ClickHouseConfig{}, fmt.Errorf("invalid ANALYTICS_CLICKHOUSE_ADDR %q", config.Addr)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return ClickHouseConfig{}, fmt.Errorf("invalid ANALYTICS_CLICKHOUSE_ADDR %q", config.Addr)
	}
	return config, nil
}
