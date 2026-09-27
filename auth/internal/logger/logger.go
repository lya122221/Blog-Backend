package logger

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

type Config struct {
	Level  string
	Format string
}

func New(writer io.Writer, config Config) (*slog.Logger, error) {
	level, err := parseLevel(config.Level)
	if err != nil {
		return nil, err
	}

	options := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(config.Format) {
	case "", "json":
		handler = slog.NewJSONHandler(writer, options)
	case "text":
		handler = slog.NewTextHandler(writer, options)
	default:
		return nil, fmt.Errorf("unsupported log format %q", config.Format)
	}

	return slog.New(handler), nil
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", value)
	}
}
