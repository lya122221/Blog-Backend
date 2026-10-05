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

func TestAuthorStatsIntegration(t *testing.T) {
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

	now := time.Now().UTC().Truncate(time.Minute)
	author, otherAuthor := uuid.New().String(), uuid.New().String()
	owned, deleted, foreign := uuid.New().String(), uuid.New().String(), uuid.New().String()
	metadata := []models.ArticleEvent{}
	for _, article := range []struct{ id, author, title string }{
		{owned, author, "Original"}, {deleted, author, "Deleted"}, {foreign, otherAuthor, "Foreign"},
	} {
		metadata = append(metadata, models.ArticleEvent{Event: models.Event{
			ID: uuid.New().String(), Type: models.ArticleCreated, ArticleID: article.id,
			AuthorID: article.author, Title: article.title, OccurredAt: now.Add(-60 * 24 * time.Hour),
		}})
	}
	metadata = append(metadata,
		models.ArticleEvent{Event: models.Event{ID: uuid.New().String(), Type: models.ArticleUpdated,
			ArticleID: owned, AuthorID: author, Title: "Current", Tags: []string{"go"}, OccurredAt: now.Add(-time.Hour)}},
		models.ArticleEvent{Event: models.Event{ID: uuid.New().String(), Type: models.ArticleDeleted,
			ArticleID: deleted, AuthorID: author, OccurredAt: now.Add(-time.Hour)}},
	)
	if err := repository.StoreBatch(ctx, metadata); err != nil {
		t.Fatalf("store metadata: %v", err)
	}
	open := models.ArticleEvent{Event: models.Event{ID: uuid.New().String(), Type: models.ArticleOpened,
		ArticleID: owned, VisitorID: uuid.New().String(), OccurredAt: now.Add(-40 * 24 * time.Hour)}, ScoreDelta: 3}
	like := models.ArticleEvent{Event: models.Event{ID: uuid.New().String(), Type: models.ArticleLiked,
		ArticleID: owned, UserID: uuid.New().String(), OccurredAt: now.Add(-24 * time.Hour)}, ScoreDelta: 6}
	impression := models.ArticleEvent{Event: models.Event{ID: uuid.New().String(), Type: models.ArticleImpression,
		ArticleID: owned, VisitorID: uuid.New().String(), OccurredAt: now.Add(-15 * time.Second)}, ScoreDelta: 1}
	events := []models.ArticleEvent{
		open, like, impression,
		{Event: models.Event{ID: uuid.New().String(), Type: models.ArticleUnliked,
			ArticleID: owned, UserID: uuid.New().String(), OccurredAt: now.Add(-10 * time.Minute)}, ScoreDelta: -6},
		{Event: models.Event{ID: uuid.New().String(), Type: models.CommentCreated,
			ArticleID: owned, UserID: uuid.New().String(), OccurredAt: now.Add(-10 * time.Minute)}, ScoreDelta: 6},
		{Event: models.Event{ID: uuid.New().String(), Type: models.ArticleLiked,
			ArticleID: deleted, UserID: uuid.New().String(), OccurredAt: now.Add(-24 * time.Hour)}, ScoreDelta: 6},
		{Event: models.Event{ID: uuid.New().String(), Type: models.ArticleLiked,
			ArticleID: foreign, UserID: uuid.New().String(), OccurredAt: now.Add(-24 * time.Hour)}, ScoreDelta: 6},
		{Event: models.Event{ID: uuid.New().String(), Type: models.ArticleImpression,
			ArticleID: owned, VisitorID: uuid.New().String(), OccurredAt: now.Add(time.Minute)}, ScoreDelta: 1},
	}
	if err := repository.StoreBatch(ctx, events); err != nil {
		t.Fatalf("store actions: %v", err)
	}
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{like}); err != nil {
		t.Fatalf("store duplicate action: %v", err)
	}

	article, err := repository.ArticleMetadata(ctx, owned)
	if err != nil || article == nil || article.AuthorID != author || article.Title != "Current" || len(article.Tags) != 1 || article.Deleted {
		t.Fatalf("article metadata = %+v, error = %v", article, err)
	}
	missing, err := repository.ArticleMetadata(ctx, uuid.New().String())
	if err != nil || missing != nil {
		t.Fatalf("missing article = %+v, error = %v", missing, err)
	}
	for _, test := range []struct {
		from  time.Time
		byDay bool
		opens int64
		score int64
	}{
		{now.Truncate(time.Hour).Add(-167 * time.Hour), false, 0, 7},
		{now.Truncate(24*time.Hour).AddDate(0, 0, -29), true, 0, 7},
		{now.Truncate(24*time.Hour).AddDate(0, 0, -89), true, 1, 10},
	} {
		buckets, err := repository.AuthorStats(ctx, author, "", test.from, now, test.byDay)
		if err != nil {
			t.Fatalf("author stats: %v", err)
		}
		var totals models.StatsCounters
		for _, bucket := range buckets {
			totals.Impressions += bucket.Impressions
			totals.Opens += bucket.Opens
			totals.Likes += bucket.Likes
			totals.Unlikes += bucket.Unlikes
			totals.Comments += bucket.Comments
			totals.Score += bucket.Score
		}
		if totals.Impressions != 1 || totals.Opens != test.opens || totals.Likes != 1 || totals.Unlikes != 1 || totals.Comments != 1 || totals.Score != test.score {
			t.Fatalf("from %s: totals = %+v, buckets = %+v", test.from, totals, buckets)
		}
	}
	buckets, err := repository.AuthorStats(ctx, author, owned, now.Truncate(24*time.Hour).AddDate(0, 0, -29), now, true)
	if err != nil || len(buckets) != 2 {
		t.Fatalf("article stats = %+v, error = %v", buckets, err)
	}
	buckets, err = repository.AuthorStats(ctx, author, foreign, now.Truncate(24*time.Hour).AddDate(0, 0, -29), now, true)
	if err != nil || len(buckets) != 0 {
		t.Fatalf("foreign article stats = %+v, error = %v", buckets, err)
	}
}
