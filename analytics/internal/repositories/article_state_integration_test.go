package repositories

import (
	"analytics/internal/config"
	"analytics/internal/models"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"
)

type articleState struct {
	authorID string
	title    string
	tags     []string
	deleted  uint8
}

func TestArticleStateIntegration(t *testing.T) {
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
	applyAnalyticsMigration(t, ctx, repository, "000001_create_article_events_table.up.sql")

	articleID := uuid.New().String()
	activeArticleID := uuid.New().String()
	authorID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	created := models.ArticleEvent{Event: models.Event{
		ID: uuid.New().String(), Type: models.ArticleCreated,
		ArticleID: articleID, AuthorID: authorID, OccurredAt: now.Add(-2 * time.Minute),
		Title: "Original", Tags: []string{"go"},
	}}
	active := models.ArticleEvent{Event: models.Event{
		ID: uuid.New().String(), Type: models.ArticleCreated,
		ArticleID: activeArticleID, AuthorID: authorID, OccurredAt: now.Add(-time.Minute),
		Title: "Active", Tags: []string{"blog"},
	}}
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{created, active}); err != nil {
		t.Fatalf("store events before migration: %v", err)
	}
	applyAnalyticsMigration(t, ctx, repository, "000005_create_article_state.up.sql")
	checkArticleState(t, ctx, repository, articleID, articleState{authorID, "Original", []string{"go"}, 0})

	updated := created
	updated.Event.ID = uuid.New().String()
	updated.Event.Type = models.ArticleUpdated
	updated.Event.OccurredAt = now.Add(-time.Minute)
	updated.Event.Title = "Updated"
	updated.Event.Tags = []string{"go", "analytics"}
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{updated}); err != nil {
		t.Fatalf("store updated article: %v", err)
	}
	checkArticleState(t, ctx, repository, articleID, articleState{authorID, "Updated", []string{"go", "analytics"}, 0})

	deleted := created
	deleted.Event.ID = uuid.New().String()
	deleted.Event.Type = models.ArticleDeleted
	deleted.Event.OccurredAt = now
	deleted.Event.Title = ""
	deleted.Event.Tags = nil
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{deleted}); err != nil {
		t.Fatalf("store deleted article: %v", err)
	}
	checkArticleState(t, ctx, repository, articleID, articleState{authorID, "", []string{}, 1})

	late := updated
	late.Event.ID = uuid.New().String()
	late.Event.Title = "Late update"
	late.Event.OccurredAt = now.Add(-30 * time.Second)
	sameTime := late
	sameTime.Event.ID = uuid.New().String()
	sameTime.Event.OccurredAt = now
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{late, sameTime, deleted}); err != nil {
		t.Fatalf("store delayed and duplicate events: %v", err)
	}
	checkArticleState(t, ctx, repository, articleID, articleState{authorID, "", []string{}, 1})

	var activeCount uint64
	if err := repository.conn.QueryRow(ctx, `SELECT count() FROM (
		SELECT article_id FROM article_state GROUP BY article_id
		HAVING tupleElement(argMaxMerge(metadata), 4) = 0)`).Scan(&activeCount); err != nil {
		t.Fatalf("query active articles: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active article count = %d, want 1", activeCount)
	}
	checkArticleState(t, ctx, repository, activeArticleID, articleState{authorID, "Active", []string{"blog"}, 0})
}

func checkArticleState(t *testing.T, ctx context.Context, repository *ClickHouse, articleID string, want articleState) {
	t.Helper()
	var got articleState
	err := repository.conn.QueryRow(ctx, `SELECT
		toString(tupleElement(argMaxMerge(metadata), 1)),
		tupleElement(argMaxMerge(metadata), 2),
		tupleElement(argMaxMerge(metadata), 3),
		tupleElement(argMaxMerge(metadata), 4)
		FROM article_state WHERE article_id = ? GROUP BY article_id`, articleID).
		Scan(&got.authorID, &got.title, &got.tags, &got.deleted)
	if err != nil {
		t.Fatalf("query article state: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("article state = %+v, want %+v", got, want)
	}
}
