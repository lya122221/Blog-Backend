package main

import (
	"blog/internal/handlers"
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

		v1.POST("/auth/register", userHandler.RegisterUser)
		v1.POST("/auth/login", userHandler.LoginUser)

		articlesService := services.NewArticlesService(storage)
		articlesHandler := handlers.NewArticlesHandler(articlesService)

		v1.GET("/articles", articlesHandler.GetArticlesHandler)
	}

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
