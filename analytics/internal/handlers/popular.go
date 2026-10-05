package handlers

import (
	"analytics/internal/models"
	"analytics/internal/services"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultPopularLimit = 10

type PopularService interface {
	Popular(context.Context, string, int) ([]models.PopularArticle, error)
}

type PopularHandler struct {
	service PopularService
	logger  *slog.Logger
}

func NewPopularHandler(service PopularService, logger *slog.Logger) *PopularHandler {
	return &PopularHandler{service: service, logger: logger}
}

func (handler *PopularHandler) Popular(c *gin.Context) {
	limit := defaultPopularLimit
	if value, exists := c.GetQuery("limit"); exists {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
			return
		}
		limit = parsed
	}
	window := c.Query("window")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	articles, err := handler.service.Popular(ctx, window, limit)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
			return
		}
		handler.logger.Error("failed to list popular articles", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "popular articles are unavailable"})
		return
	}
	c.JSON(http.StatusOK, models.PopularResponse{Window: window, Articles: articles})
}
