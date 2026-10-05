package services

import (
	"analytics/internal/models"
	"context"
	"errors"
	"testing"
	"time"
)

type popularRepositoryStub struct {
	since    time.Time
	until    time.Time
	limit    int
	calls    int
	articles []models.PopularArticle
	err      error
}

func (stub *popularRepositoryStub) ListPopular(_ context.Context, since, until time.Time, limit int) ([]models.PopularArticle, error) {
	stub.since, stub.until, stub.limit = since, until, limit
	stub.calls++
	return stub.articles, stub.err
}

func TestPopularUsesRequestedWindowAndLimit(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 30, 45, 123456789, time.UTC)
	for window, duration := range map[string]time.Duration{"5m": 5 * time.Minute, "1h": time.Hour, "1d": 24 * time.Hour} {
		t.Run(window, func(t *testing.T) {
			stub := &popularRepositoryStub{articles: []models.PopularArticle{{ArticleID: "article-1", Score: 9}}}
			service := &PopularService{repository: stub, now: func() time.Time { return now }}
			articles, err := service.Popular(context.Background(), window, 25)
			until := now.Truncate(time.Microsecond)
			if err != nil || len(articles) != 1 || stub.calls != 1 || stub.limit != 25 ||
				!stub.since.Equal(until.Add(-duration)) || !stub.until.Equal(until) {
				t.Fatalf("articles = %+v, error = %v, repository = %+v", articles, err, stub)
			}
		})
	}
}

func TestPopularRejectsInvalidParameters(t *testing.T) {
	for _, test := range []struct {
		window string
		limit  int
	}{
		{"", 10}, {"10m", 10}, {"5m", 0}, {"5m", 101},
	} {
		stub := &popularRepositoryStub{}
		service := NewPopularService(stub)
		_, err := service.Popular(context.Background(), test.window, test.limit)
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) || stub.calls != 0 {
			t.Fatalf("window = %q, limit = %d, error = %v, calls = %d", test.window, test.limit, err, stub.calls)
		}
	}
}

func TestPopularPropagatesRepositoryFailure(t *testing.T) {
	failure := errors.New("ClickHouse unavailable")
	stub := &popularRepositoryStub{err: failure}
	_, err := NewPopularService(stub).Popular(context.Background(), "5m", 10)
	if !errors.Is(err, failure) {
		t.Fatalf("Popular error = %v", err)
	}
}
