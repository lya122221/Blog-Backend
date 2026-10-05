package handlers

import (
	"analytics/internal/models"
	"analytics/internal/services"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"
	"uuid"

	"github.com/gin-gonic/gin"
)

const (
	cookieName   = "analytics_visitor"
	cookieAge    = 90 * 24 * time.Hour
	maxBodyBytes = 64 << 10
)

type ViewsService interface {
	RecordViews(context.Context, models.ViewRequest, string) (int, error)
}

type ViewsHandler struct {
	service      ViewsService
	logger       *slog.Logger
	secureCookie bool
}

func NewViewsHandler(service ViewsService, logger *slog.Logger, secureCookie bool) *ViewsHandler {
	return &ViewsHandler{service: service, logger: logger, secureCookie: secureCookie}
}

func (handler *ViewsHandler) Views(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var request models.ViewRequest
	if err := decoder.Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	visitorID := handler.visitorID(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	accepted, err := handler.service.RecordViews(ctx, request, visitorID)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
			return
		}
		handler.logger.Error("failed to publish view events", "error", err)
		c.Header("Retry-After", "1")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "events could not be accepted; retry with the same event IDs"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"accepted": accepted})
}

func (handler *ViewsHandler) visitorID(c *gin.Context) string {
	if cookie, err := c.Request.Cookie(cookieName); err == nil {
		if parsed, err := uuid.Parse(cookie.Value); err == nil {
			return parsed.String()
		}
	}
	id := uuid.New().String()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   int(cookieAge.Seconds()),
		HttpOnly: true,
		Secure:   handler.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
	return id
}
