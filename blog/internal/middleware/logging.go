package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	requestIDHeader = "X-Request-ID"
	requestIDKey    = "requestID"
)

func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		requestID := c.GetHeader(requestIDHeader)
		if requestID == "" || len(requestID) > 128 {
			requestID = uuid.NewString()
		}

		c.Set(requestIDKey, requestID)
		c.Header(requestIDHeader, requestID)
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		attributes := []slog.Attr{
			slog.String("request_id", requestID),
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
			slog.Int("response_size", c.Writer.Size()),
		}

		if userID, exists := c.Get("userID"); exists {
			if value, ok := userID.(string); ok {
				attributes = append(attributes, slog.String("user_id", value))
			}
		}
		if len(c.Errors) > 0 {
			attributes = append(attributes, slog.String("errors", c.Errors.String()))
		}

		logger.LogAttrs(
			c.Request.Context(),
			requestLogLevel(c.Writer.Status()),
			"http request",
			attributes...,
		)
	}
}

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		requestID, _ := c.Get(requestIDKey)
		logger.ErrorContext(
			c.Request.Context(),
			"panic recovered",
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"panic", recovered,
			"stack", string(debug.Stack()),
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
	})
}

func requestLogLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
