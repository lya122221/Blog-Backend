package main

import (
	applog "auth/internal/logger"
	"auth/internal/middleware"
	"auth/internal/repositories"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	_ = godotenv.Load()

	logger, err := applog.New(os.Stdout, applog.Config{
		Level:  os.Getenv("LOG_LEVEL"),
		Format: os.Getenv("LOG_FORMAT"),
	})
	if err != nil {
		return fmt.Errorf("configure logger: %w", err)
	}
	slog.SetDefault(logger)

	pgHost := os.Getenv("AUTH_POSTGRES_HOST")
	if pgHost == "" {
		pgHost = "localhost"
	}

	pgDSN := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable",
		os.Getenv("AUTH_POSTGRES_USER"),
		os.Getenv("AUTH_POSTGRES_PASSWORD"),
		pgHost,
		os.Getenv("AUTH_POSTGRES_DB"),
	)

	storage, err := repositories.NewStorage(pgDSN)
	if err != nil {
		return fmt.Errorf("connect to storage: %w", err)
	}
	defer func() {
		runErr = errors.Join(runErr, storage.Close())
		if runErr == nil {
			logger.Info("application stopped")
		}
	}()

	r := gin.New()
	r.Use(middleware.RequestLogger(logger), middleware.Recovery(logger))

	server := &http.Server{
		Addr:              ":8081",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen HTTP: %w", err)
	}

	signalCtx, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()

	return serveHTTP(signalCtx, server, listener, logger)
}

func serveHTTP(ctx context.Context, server *http.Server, listener net.Listener, logger *slog.Logger) error {
	serverDone := make(chan error, 1)
	logger.Info("starting HTTP server", "address", listener.Addr().String())
	go func() {
		serverDone <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()

		var resultErr error
		if err := server.Shutdown(shutdownCtx); err != nil {
			resultErr = fmt.Errorf("shut down HTTP server: %w", err)
			if closeErr := server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
				resultErr = errors.Join(resultErr, fmt.Errorf("force close HTTP server: %w", closeErr))
			}
		}

		if serverErr := <-serverDone; serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
			resultErr = errors.Join(resultErr, fmt.Errorf("serve HTTP: %w", serverErr))
		}
		return resultErr

	case serverErr := <-serverDone:
		if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", serverErr)
		}
		return nil
	}
}
