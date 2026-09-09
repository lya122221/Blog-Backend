package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func testLogger(output *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, nil))
}

func TestRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	router := gin.New()
	router.Use(RequestLogger(testLogger(&output)))
	router.GET("/articles/:id", func(c *gin.Context) {
		c.Set("userID", "user-1")
		c.Status(http.StatusCreated)
	})

	request := httptest.NewRequest(http.MethodGet, "/articles/article-1", nil)
	request.Header.Set(requestIDHeader, "request-123")
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Header().Get(requestIDHeader) != "request-123" {
		t.Fatalf("request ID header = %q", response.Header().Get(requestIDHeader))
	}

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log: %v; output=%s", err, output.String())
	}
	if record["msg"] != "http request" ||
		record["request_id"] != "request-123" ||
		record["method"] != http.MethodGet ||
		record["path"] != "/articles/:id" ||
		record["user_id"] != "user-1" {
		t.Fatalf("unexpected log record: %+v", record)
	}
	if record["status"] != float64(http.StatusCreated) {
		t.Fatalf("logged status = %v", record["status"])
	}
}

func TestRequestLoggerGeneratesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	router := gin.New()
	router.Use(RequestLogger(testLogger(&output)))
	router.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	requestID := response.Header().Get(requestIDHeader)
	if requestID == "" || !strings.Contains(output.String(), requestID) {
		t.Fatalf("generated request ID is missing: header=%q log=%s", requestID, output.String())
	}
}

func TestRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	router := gin.New()
	log := testLogger(&output)
	router.Use(RequestLogger(log), Recovery(log))
	router.GET("/panic", func(*gin.Context) {
		panic("boom")
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(output.String(), "panic recovered") ||
		!strings.Contains(output.String(), `"level":"ERROR"`) ||
		!strings.Contains(output.String(), `"status":500`) {
		t.Fatalf("unexpected recovery log: %s", output.String())
	}
}

func TestRequestLogLevel(t *testing.T) {
	tests := []struct {
		status int
		level  slog.Level
	}{
		{http.StatusOK, slog.LevelInfo},
		{http.StatusBadRequest, slog.LevelWarn},
		{http.StatusInternalServerError, slog.LevelError},
	}

	for _, test := range tests {
		if got := requestLogLevel(test.status); got != test.level {
			t.Fatalf("requestLogLevel(%d)=%s want=%s", test.status, got, test.level)
		}
	}
}
