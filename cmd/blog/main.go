package main

import (
	"blog/internal/handlers"
	"blog/internal/middleware"
	"blog/internal/repositories"
	"blog/internal/services"
	"blog/internal/workers"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

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
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer storage.Close()

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go workers.StartViewsUpdaterWorker(ctx, storage)

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
