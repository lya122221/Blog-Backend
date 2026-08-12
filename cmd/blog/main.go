package main

import (
	"blog/internal/handlers"
	"blog/internal/middleware"
	"blog/internal/repositories"
	"blog/internal/services"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	dsn := fmt.Sprintf("postgres://%s:%s@localhost:5432/%s?sslmode=disable",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_DB"),
	)

	storage, err := repositories.NewStorage(dsn)
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

		articles := v1.Group("/articles")
		articles.Use(middleware.AuthMiddleware())
		{
			articles.GET("/", articlesHandler.GetArticlesHandler)
			articles.POST("/", articlesHandler.CreateArticlesHandler)
		}
	}

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
