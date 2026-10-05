package handlers

import (
	"analytics/internal/models"
	"analytics/internal/services"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type AuthorStatsService interface {
	AuthorStats(context.Context, string, string) (models.AuthorStats, error)
	ArticleStats(context.Context, string, string, string) (models.ArticleStats, error)
}

type AuthorStatsHandler struct {
	service AuthorStatsService
	logger  *slog.Logger
}

func NewAuthorStatsHandler(service AuthorStatsService, logger *slog.Logger) *AuthorStatsHandler {
	return &AuthorStatsHandler{service: service, logger: logger}
}

func (handler *AuthorStatsHandler) AuthorStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	stats, err := handler.service.AuthorStats(ctx, c.GetString("userID"), c.Query("period"))
	if err != nil {
		handler.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (handler *AuthorStatsHandler) ArticleStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	stats, err := handler.service.ArticleStats(ctx, c.GetString("userID"), c.Param("id"), c.Query("period"))
	if err != nil {
		handler.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (handler *AuthorStatsHandler) respondError(c *gin.Context, err error) {
	var validationErr *services.ValidationError
	switch {
	case errors.As(err, &validationErr):
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
	case errors.Is(err, services.ErrArticleNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "article not found"})
	default:
		handler.logger.Error("failed to read author stats", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "author stats are unavailable"})
	}
}
