package services

import (
	"analytics/internal/models"
	"context"
	"errors"
	"testing"
	"time"
)

type authorStatsRepositoryStub struct {
	article   *models.ArticleMetadata
	rows      []models.StatsBucket
	err       error
	from      time.Time
	to        time.Time
	byDay     bool
	author    string
	articleID string
	calls     int
}

func (stub *authorStatsRepositoryStub) ArticleMetadata(context.Context, string) (*models.ArticleMetadata, error) {
	return stub.article, stub.err
}

func (stub *authorStatsRepositoryStub) AuthorStats(_ context.Context, authorID, articleID string, from, to time.Time, byDay bool) ([]models.StatsBucket, error) {
	stub.author, stub.articleID, stub.from, stub.to, stub.byDay = authorID, articleID, from, to, byDay
	stub.calls++
	return stub.rows, stub.err
}

func TestAuthorStatsPeriodsAndZeroBuckets(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 45, 31, 0, time.UTC)
	for _, test := range []struct {
		period string
		count  int
		step   time.Duration
		from   time.Time
		byDay  bool
	}{
		{"7d", 168, time.Hour, time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC), false},
		{"30d", 30, 24 * time.Hour, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC), true},
		{"90d", 90, 24 * time.Hour, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), true},
	} {
		t.Run(test.period, func(t *testing.T) {
			stub := &authorStatsRepositoryStub{rows: []models.StatsBucket{{
				Start: test.from.Add(test.step), StatsCounters: models.StatsCounters{Impressions: 2, Opens: 1, Likes: 1, Score: 11},
			}}}
			service := NewAuthorStatsService(stub)
			service.now = func() time.Time { return now }
			result, err := service.AuthorStats(context.Background(), "author", test.period)
			if err != nil {
				t.Fatal(err)
			}
			if stub.calls != 1 || stub.author != "author" || stub.articleID != "" || !stub.from.Equal(test.from) || !stub.to.Equal(now) || stub.byDay != test.byDay {
				t.Fatalf("repository call = %+v", stub)
			}
			if result.Period != test.period || !result.From.Equal(test.from) || !result.To.Equal(now) || len(result.Series) != test.count ||
				!result.Series[0].Start.Equal(test.from) || result.Series[0].Score != 0 || result.Series[1].Score != 11 ||
				result.Totals.Impressions != 2 || result.Totals.Score != 11 {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestArticleStatsRequiresOwnershipAndActiveArticle(t *testing.T) {
	const articleID = "55555555-5555-4555-8555-555555555555"
	for _, article := range []*models.ArticleMetadata{
		nil,
		{AuthorID: "other", Title: "Private"},
		{AuthorID: "author", Deleted: true},
	} {
		stub := &authorStatsRepositoryStub{article: article}
		_, err := NewAuthorStatsService(stub).ArticleStats(context.Background(), "author", articleID, "7d")
		if !errors.Is(err, ErrArticleNotFound) || stub.calls != 0 {
			t.Fatalf("article = %+v, error = %v, calls = %d", article, err, stub.calls)
		}
	}
	stub := &authorStatsRepositoryStub{article: &models.ArticleMetadata{AuthorID: "author", Title: "My article", Tags: []string{"go"}}}
	result, err := NewAuthorStatsService(stub).ArticleStats(context.Background(), "author", articleID, "30d")
	if err != nil || result.ArticleID != articleID || result.Title != "My article" || len(result.Tags) != 1 || len(result.Series) != 30 || stub.articleID != articleID {
		t.Fatalf("result = %+v, error = %v, repository = %+v", result, err, stub)
	}
}

func TestAuthorStatsRejectsInvalidInputAndPropagatesStorageError(t *testing.T) {
	stub := &authorStatsRepositoryStub{}
	service := NewAuthorStatsService(stub)
	if _, err := service.AuthorStats(context.Background(), "author", "all"); err == nil || stub.calls != 0 {
		t.Fatalf("invalid period error = %v, calls = %d", err, stub.calls)
	}
	if _, err := service.ArticleStats(context.Background(), "author", "not-a-uuid", "7d"); err == nil || stub.calls != 0 {
		t.Fatalf("invalid article ID error = %v, calls = %d", err, stub.calls)
	}
	stub.err = errors.New("ClickHouse failed")
	if _, err := service.AuthorStats(context.Background(), "author", "7d"); !errors.Is(err, stub.err) {
		t.Fatalf("storage error = %v", err)
	}
}
