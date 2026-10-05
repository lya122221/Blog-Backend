package repositories

import (
	"analytics/internal/config"
	"analytics/internal/models"
	"context"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestListPopularIntegration(t *testing.T) {
	addr := os.Getenv("ANALYTICS_TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		t.Skip("set ANALYTICS_TEST_CLICKHOUSE_ADDR to run ClickHouse integration test")
	}
	settings := config.ClickHouseConfig{
		Addr: addr, Database: os.Getenv("ANALYTICS_TEST_CLICKHOUSE_DATABASE"),
		User: os.Getenv("ANALYTICS_TEST_CLICKHOUSE_USER"), Password: os.Getenv("ANALYTICS_TEST_CLICKHOUSE_PASSWORD"),
	}
	if settings.Database == "" || settings.User == "" {
		t.Fatal("set ANALYTICS_TEST_CLICKHOUSE_DATABASE and ANALYTICS_TEST_CLICKHOUSE_USER")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := NewClickHouse(ctx, settings)
	if err != nil {
		t.Fatalf("connect to ClickHouse: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	database := "analytics_test_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if err := admin.conn.Exec(ctx, "CREATE DATABASE "+database); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := admin.conn.Exec(cleanupCtx, "DROP DATABASE "+database+" SYNC"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})
	settings.Database = database
	repository, err := NewClickHouse(ctx, settings)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	defer func() { _ = repository.Close() }()
	for _, migration := range []string{
		"000001_create_article_events_table.up.sql",
		"000002_create_article_stats_minute.up.sql",
		"000003_create_article_stats_hour.up.sql",
		"000004_create_article_stats_day.up.sql",
		"000005_create_article_state.up.sql",
	} {
		applyAnalyticsMigration(t, ctx, repository, migration)
	}

	until := time.Now().UTC().Truncate(time.Minute).Add(-25 * time.Second)
	since := until.Add(-5 * time.Minute)
	authorID := uuid.New().String()
	articleA, articleB, articleC, articleD := uuid.New().String(), uuid.New().String(), uuid.New().String(), uuid.New().String()
	metadata := make([]models.ArticleEvent, 0, 6)
	for _, article := range []struct{ id, title string }{
		{articleA, "Original"}, {articleB, "Second"}, {articleC, "Deleted"}, {articleD, "Negative"},
	} {
		metadata = append(metadata, models.ArticleEvent{Event: models.Event{
			ID: uuid.New().String(), Type: models.ArticleCreated,
			ArticleID: article.id, AuthorID: authorID, OccurredAt: until.Add(-4 * time.Hour), Title: article.title,
		}})
	}
	metadata = append(metadata,
		models.ArticleEvent{Event: models.Event{
			ID: uuid.New().String(), Type: models.ArticleUpdated,
			ArticleID: articleA, AuthorID: authorID, OccurredAt: until.Add(-time.Minute),
			Title: "Current", Tags: []string{"go", "popular"},
		}},
		models.ArticleEvent{Event: models.Event{
			ID: uuid.New().String(), Type: models.ArticleDeleted,
			ArticleID: articleC, AuthorID: authorID, OccurredAt: until.Add(-time.Minute),
		}},
	)
	if err := repository.StoreBatch(ctx, metadata); err != nil {
		t.Fatalf("store article metadata: %v", err)
	}

	action := func(articleID string, eventType models.EventType, occurredAt time.Time, delta int16) models.ArticleEvent {
		return models.ArticleEvent{Event: models.Event{
			ID: uuid.New().String(), Type: eventType, ArticleID: articleID,
			UserID: uuid.New().String(), VisitorID: uuid.New().String(), OccurredAt: occurredAt,
		}, ScoreDelta: delta}
	}
	duplicate := action(articleB, models.ArticleOpened, until.Add(-3*time.Minute), 3)
	events := []models.ArticleEvent{
		action(articleA, models.ArticleImpression, since.Add(-time.Microsecond), 1),
		action(articleA, models.ArticleImpression, since, 1),
		action(articleA, models.ArticleOpened, since.Add(10*time.Second), 3),
		action(articleA, models.ArticleLiked, since.Add(45*time.Second), 6),
		action(articleA, models.CommentCreated, until.Add(-10*time.Second), 6),
		action(articleA, models.ArticleImpression, until, 1),
		action(articleA, models.ArticleOpened, until.Add(-2*time.Hour), 3),
		duplicate,
		action(articleB, models.ArticleLiked, until.Add(-2*time.Minute), 6),
		action(articleB, models.ArticleLiked, until.Add(-30*time.Minute), 6),
		action(articleC, models.ArticleLiked, until.Add(-2*time.Minute), 6),
		action(articleD, models.ArticleUnliked, until.Add(-2*time.Minute), -6),
	}
	if err := repository.StoreBatch(ctx, events); err != nil {
		t.Fatalf("store article actions: %v", err)
	}
	duplicate.Event.Title = "duplicate delivery"
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{duplicate}); err != nil {
		t.Fatalf("store duplicate action: %v", err)
	}

	for _, test := range []struct {
		window time.Duration
		scoreA int64
		scoreB int64
	}{
		{5 * time.Minute, 16, 9},
		{time.Hour, 17, 15},
		{24 * time.Hour, 20, 15},
	} {
		articles, err := repository.ListPopular(ctx, until.Add(-test.window), until, 10)
		if err != nil {
			t.Fatalf("window %s: %v", test.window, err)
		}
		if len(articles) != 3 || articles[0].ArticleID != articleA || articles[0].Score != test.scoreA ||
			articles[0].Title != "Current" || len(articles[0].Tags) != 2 || articles[1].ArticleID != articleB ||
			articles[1].Score != test.scoreB || articles[2].ArticleID != articleD || articles[2].Score != -6 {
			t.Fatalf("window %s: articles = %+v", test.window, articles)
		}
	}
	articles, err := repository.ListPopular(ctx, since, until, 1)
	if err != nil || len(articles) != 1 || articles[0].ArticleID != articleA {
		t.Fatalf("limited articles = %+v, error = %v", articles, err)
	}
}
