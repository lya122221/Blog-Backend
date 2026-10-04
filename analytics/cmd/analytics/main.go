package main

import (
	"analytics/internal/config"
	"analytics/internal/consumer"
	"analytics/internal/handlers"
	"analytics/internal/logger"
	"analytics/internal/middleware"
	"analytics/internal/models"
	"analytics/internal/producer"
	"analytics/internal/repositories"
	"analytics/internal/services"
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
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	eventsService, err := services.NewEventsService(models.DefaultWeights())
	if err != nil {
		return fmt.Errorf("configure event scoring: %w", err)
	}
	storage, err := repositories.NewClickHouse(signalCtx, settings.ClickHouse)
	if err != nil {
		return fmt.Errorf("connect to ClickHouse: %w", err)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			appLogger.Error("close ClickHouse connection", "error", err)
		}
	}()
	deadLetter, err := producer.NewDeadLetter(settings.KafkaBrokers, settings.KafkaDLQTopic)
	if err != nil {
		return fmt.Errorf("configure Kafka DLQ producer: %w", err)
	}
	defer deadLetter.Close()
	worker, err := consumer.NewConsumer(settings, eventsService, storage, deadLetter)
	if err != nil {
		return fmt.Errorf("configure Kafka consumer: %w", err)
	}
	defer worker.Close()

	eventProducer, err := producer.NewProducer(settings.KafkaBrokers, settings.KafkaTopic)
	if err != nil {
		return fmt.Errorf("configure Kafka producer: %w", err)
	}
	defer eventProducer.Close()
	viewService := services.NewViewsService(eventProducer)
	viewHandler := handlers.NewViewsHandler(viewService, appLogger, settings.CookieSecure)

	server := &http.Server{
		Addr:              settings.Address(),
		Handler:           newRouter(appLogger, viewHandler.Views),
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen HTTP: %w", err)
	}

	return serveAnalytics(signalCtx, server, listener, worker, appLogger)
}

type eventConsumer interface {
	Run(context.Context) error
}

func serveAnalytics(ctx context.Context, server *http.Server, listener net.Listener, worker eventConsumer, appLogger *slog.Logger) error {
	serviceCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpDone := make(chan error, 1)
	workerDone := make(chan error, 1)
	go func() { httpDone <- serveHTTP(serviceCtx, server, listener, appLogger) }()
	go func() { workerDone <- worker.Run(serviceCtx) }()

	select {
	case httpErr := <-httpDone:
		cancel()
		return errors.Join(httpErr, <-workerDone)
	case workerErr := <-workerDone:
		if workerErr == nil && ctx.Err() == nil {
			workerErr = errors.New("Kafka consumer stopped unexpectedly")
		}
		cancel()
		return errors.Join(workerErr, <-httpDone)
	}
}

func newRouter(appLogger *slog.Logger, handleViews gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use(middleware.RequestLogger(appLogger), middleware.Recovery(appLogger))
	router.POST("/api/v1/analytics/views", handleViews)
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
