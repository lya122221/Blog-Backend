package main

import (
	"analytics/internal/config"
	"analytics/internal/logger"
	"analytics/internal/middleware"
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

func run() error {
	_ = godotenv.Load()

	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("configure analytics service: %w", err)
	}
	appLogger, err := logger.New(os.Stdout, logger.Config{
		Level:  settings.LogLevel,
		Format: settings.LogFormat,
	})
	if err != nil {
		return fmt.Errorf("configure logger: %w", err)
	}
	slog.SetDefault(appLogger)

	server := &http.Server{
		Addr:              settings.Address(),
		Handler:           newRouter(appLogger),
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen HTTP: %w", err)
	}

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	return serveHTTP(signalCtx, server, listener, appLogger)
}

func newRouter(appLogger *slog.Logger) *gin.Engine {
	router := gin.New()
	router.Use(middleware.RequestLogger(appLogger), middleware.Recovery(appLogger))
	return router
}

func serveHTTP(ctx context.Context, server *http.Server, listener net.Listener, appLogger *slog.Logger) error {
	serverDone := make(chan error, 1)
	appLogger.Info("starting HTTP server", "address", listener.Addr().String())
	go func() {
		serverDone <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		appLogger.Info("shutdown signal received")
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
