package handlers

import (
	"analytics/internal/models"
	"analytics/internal/services"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type popularServiceStub struct {
	window   string
	limit    int
	calls    int
	articles []models.PopularArticle
	err      error
}

func (stub *popularServiceStub) Popular(_ context.Context, window string, limit int) ([]models.PopularArticle, error) {
	stub.window, stub.limit = window, limit
	stub.calls++
	return stub.articles, stub.err
}

func sendPopular(stub *popularServiceStub, path string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	router := gin.New()
	router.GET("/api/v1/analytics/popular", NewPopularHandler(stub, logger).Popular)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func TestPopularReturnsArticlesAndUsesDefaultLimit(t *testing.T) {
	stub := &popularServiceStub{articles: []models.PopularArticle{{
		ArticleID: "article-1", Title: "Go", Tags: []string{"backend"}, Score: 9,
	}}}
	response := sendPopular(stub, "/api/v1/analytics/popular?window=5m")
	if response.Code != http.StatusOK || stub.window != "5m" || stub.limit != defaultPopularLimit || stub.calls != 1 {
		t.Fatalf("response = %d, service = %+v", response.Code, stub)
	}
	var body models.PopularResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Window != "5m" || len(body.Articles) != 1 || body.Articles[0].Title != "Go" || body.Articles[0].Score != 9 {
		t.Fatalf("response body = %+v", body)
	}
}

func TestPopularPassesRequestedLimit(t *testing.T) {
	stub := &popularServiceStub{articles: []models.PopularArticle{}}
	response := sendPopular(stub, "/api/v1/analytics/popular?window=1h&limit=25")
	if response.Code != http.StatusOK || stub.limit != 25 || response.Body.String() != `{"window":"1h","articles":[]}` {
		t.Fatalf("response = %d, body = %s, limit = %d", response.Code, response.Body.String(), stub.limit)
	}
}

func TestPopularRejectsInvalidLimit(t *testing.T) {
	for _, value := range []string{"", "many"} {
		stub := &popularServiceStub{}
		response := sendPopular(stub, "/api/v1/analytics/popular?window=5m&limit="+value)
		if response.Code != http.StatusBadRequest || stub.calls != 0 {
			t.Fatalf("limit = %q, response = %d, calls = %d", value, response.Code, stub.calls)
		}
	}
}

func TestPopularMapsServiceErrors(t *testing.T) {
	for _, test := range []struct {
		err        error
		wantStatus int
	}{
		{&services.ValidationError{}, http.StatusBadRequest},
		{errors.New("ClickHouse unavailable"), http.StatusServiceUnavailable},
	} {
		stub := &popularServiceStub{err: test.err}
		response := sendPopular(stub, "/api/v1/analytics/popular?window=5m")
		if response.Code != test.wantStatus || stub.calls != 1 {
			t.Fatalf("error = %v, response = %d, calls = %d", test.err, response.Code, stub.calls)
		}
	}
}
