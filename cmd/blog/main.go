package main

import (
	"blog/internal/handlers"
	"blog/internal/middleware"
	"blog/internal/repositories"
	"blog/internal/services"
	"blog/internal/workers"
	"context"
	"errors"
	"fmt"
	"log"
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
		log.Printf("application stopped with an error: %v", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	_ = godotenv.Load()

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
	}()

	r := gin.Default()

	v1 := r.Group("/api/v1")
	{
		userService := services.NewUserService(storage)
		userHandler := handlers.NewUserHandler(userService)

		auth := v1.Group("/auth")
		{
			auth.POST("/register", userHandler.RegisterUser)
			auth.POST("/login", userHandler.LoginUser)
		}

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
			articles.Use(middleware.AuthMiddleware())
			articles.POST("/", articlesHandler.CreateArticlesHandler)
			articles.PUT("/:id", articlesHandler.UpdateArticleHandler)
			articles.DELETE("/:id", articlesHandler.DeleteArticleHandler)
			articles.POST("/:id/comments", interactionsHandler.CreateCommentHandler)
			articles.POST("/:id/like", interactionsHandler.ToggleLikeHandler)
		}
	}

	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan error, 1)

	go func() {
		workerDone <- workers.StartViewsUpdaterWorker(workerCtx, storage)
	}()

	server := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverDone := make(chan error, 1)
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
		log.Println("shutdown signal received")
		runErr = errors.Join(runErr, shutdownHTTPServer(server, serverDone, 10*time.Second))

	case serverErr := <-serverDone:
		if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
			runErr = errors.Join(runErr, fmt.Errorf("serve HTTP: %w", serverErr))
		}
	}

	stopWorker()
	runErr = errors.Join(runErr, waitForWorker(workerDone, 5*time.Second))

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

func waitForWorker(workerDone <-chan error, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-workerDone:
		if err != nil {
			return fmt.Errorf("stop views updater: %w", err)
		}
		return nil
	case <-timer.C:
		return errors.New("timed out waiting for views updater")
	}
}
