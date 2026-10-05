package main

import (
	"blog/internal/config"
	"blog/internal/handlers"
	applog "blog/internal/logger"
	"blog/internal/middleware"
	"blog/internal/producer"
	"blog/internal/repositories"
	"blog/internal/services"
	"blog/internal/workers"
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	jwtMiddleware, err := middleware.AuthMiddleware(os.Getenv("BLOG_JWT_PUBLIC_KEY"))
	if err != nil {
		return fmt.Errorf("configure JWT middleware: %w", err)
	}
	outboxConfig, err := config.LoadOutbox()
	if err != nil {
		return fmt.Errorf("configure outbox: %w", err)
	}

	pgHost := os.Getenv("POSTGRES_HOST")
	if pgHost == "" {
		pgHost = "localhost"
	}

	pgDSN := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		pgHost,
		os.Getenv("POSTGRES_DB"),
	)

	redisDSN := os.Getenv("REDIS_HOST")
	if redisDSN == "" {
		redisDSN = "localhost"
	}
	redisDSN = redisDSN + ":6379"

	storage, err := repositories.NewStorage(pgDSN, redisDSN)
	if err != nil {
		return fmt.Errorf("connect to storage: %w", err)
	}
	defer func() {
		runErr = errors.Join(runErr, storage.Close())
		if runErr == nil {
			logger.Info("application stopped")
		}
	}()
	outboxProducer, err := producer.NewProducer(outboxConfig.KafkaBrokers, outboxConfig.KafkaTopic)
	if err != nil {
		return fmt.Errorf("create outbox producer: %w", err)
	}
	defer outboxProducer.Close()

	r := gin.New()
	r.Use(middleware.RequestLogger(logger), middleware.Recovery(logger))

	v1 := r.Group("/api/v1")
	{
		articlesService := services.NewArticlesService(storage)
		articlesHandler := handlers.NewArticlesHandler(articlesService)

		interactionsService := services.NewInteractionsService(storage)
		interactionsHandler := handlers.NewInteractionsHandler(interactionsService)

		articles := v1.Group("/articles")
		{
			// public
			articles.GET("/", articlesHandler.GetArticlesHandler)
			articles.GET("/:id", articlesHandler.GetArticleWithIDHandler)
			articles.GET("/:id/comments", interactionsHandler.GetCommentsHandler)

			// private
			articles.Use(jwtMiddleware)
			articles.POST("/", articlesHandler.CreateArticlesHandler)
			articles.PUT("/:id", articlesHandler.UpdateArticleHandler)
			articles.DELETE("/:id", articlesHandler.DeleteArticleHandler)
			articles.POST("/:id/comments", interactionsHandler.CreateCommentHandler)
			articles.POST("/:id/like", interactionsHandler.ToggleLikeHandler)
		}
	}

	workerCtx, stopWorker := context.WithCancel(context.Background())
	viewsWorkerDone := make(chan error, 1)
	outboxWorkerDone := make(chan error, 1)

	go func() {
		viewsWorkerDone <- workers.StartViewsUpdaterWorker(workerCtx, storage)
	}()
	outboxWorker := workers.NewOutboxWorker(storage, outboxProducer, logger, outboxConfig)
	go func() {
		outboxWorkerDone <- outboxWorker.Run(workerCtx)
	}()

	server := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverDone := make(chan error, 1)
	logger.Info("starting HTTP server", "address", server.Addr)
	go func() {
		serverDone <- server.ListenAndServe()
	}()

	signalCtx, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()

	select {
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
		runErr = errors.Join(runErr, shutdownHTTPServer(server, serverDone, 10*time.Second))

	case serverErr := <-serverDone:
		if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
			runErr = errors.Join(runErr, fmt.Errorf("serve HTTP: %w", serverErr))
		}
	}

	stopWorker()
	workersDeadline := time.Now().Add(10 * time.Second)
	runErr = errors.Join(runErr, waitForWorker("views updater", viewsWorkerDone, time.Until(workersDeadline)))
	runErr = errors.Join(runErr, waitForWorker("outbox publisher", outboxWorkerDone, time.Until(workersDeadline)))

	return runErr
}

type gracefulHTTPServer interface {
	Shutdown(context.Context) error
	Close() error
}

func shutdownHTTPServer(server gracefulHTTPServer, serverDone <-chan error, timeout time.Duration) error {
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), timeout)
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
}

func waitForWorker(name string, workerDone <-chan error, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-workerDone:
		if err != nil {
			return fmt.Errorf("stop %s: %w", name, err)
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("timed out waiting for %s", name)
	}
}
