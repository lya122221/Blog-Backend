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

type authorStatsServiceStub struct {
	authorID  string
	articleID string
	period    string
	result    models.AuthorStats
	article   models.ArticleStats
	err       error
}

func (stub *authorStatsServiceStub) AuthorStats(_ context.Context, authorID, period string) (models.AuthorStats, error) {
	stub.authorID, stub.period = authorID, period
	return stub.result, stub.err
}

func (stub *authorStatsServiceStub) ArticleStats(_ context.Context, authorID, articleID, period string) (models.ArticleStats, error) {
	stub.authorID, stub.articleID, stub.period = authorID, articleID, period
	return stub.article, stub.err
}

func TestAuthorStatsHandlerRoutesAndErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		path   string
		err    error
		status int
	}{
		{"/api/v1/analytics/me/stats?period=7d", nil, http.StatusOK},
		{"/api/v1/analytics/me/articles/article-1/stats?period=30d", nil, http.StatusOK},
		{"/api/v1/analytics/me/stats?period=all", &services.ValidationError{}, http.StatusBadRequest},
		{"/api/v1/analytics/me/articles/article-1/stats?period=7d", services.ErrArticleNotFound, http.StatusNotFound},
		{"/api/v1/analytics/me/stats?period=7d", errors.New("storage failed"), http.StatusServiceUnavailable},
	} {
		stub := &authorStatsServiceStub{err: test.err}
		logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		handler := NewAuthorStatsHandler(stub, logger)
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("userID", "author-1") })
		router.GET("/api/v1/analytics/me/stats", handler.AuthorStats)
		router.GET("/api/v1/analytics/me/articles/:id/stats", handler.ArticleStats)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.status || stub.authorID != "author-1" {
			t.Fatalf("path %s: response = %d %s, stub = %+v", test.path, response.Code, response.Body.String(), stub)
		}
		if stub.articleID != "" && stub.articleID != "article-1" {
			t.Fatalf("article ID = %q", stub.articleID)
		}
	}
}

func TestArticleStatsHandlerResponseContainsMetadataAndCounters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &authorStatsServiceStub{article: models.ArticleStats{
		ArticleID: "article-1", Title: "My article", Tags: []string{"go"},
		AuthorStats: models.AuthorStats{
			Period: "7d", Totals: models.StatsCounters{Impressions: 2, Score: 2},
			Series: []models.StatsBucket{},
		},
	}}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	router := gin.New()
	router.GET("/api/v1/analytics/me/articles/:id/stats", NewAuthorStatsHandler(stub, logger).ArticleStats)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/analytics/me/articles/article-1/stats?period=7d", nil))
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || string(body["article_id"]) != `"article-1"` || string(body["title"]) != `"My article"` ||
		string(body["period"]) != `"7d"` || string(body["series"]) != `[]` {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}
