package repositories

import (
	"analytics/internal/config"
	"analytics/internal/models"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestArticleStatsIntegration(t *testing.T) {
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

	applyStatsMigration(t, ctx, repository, "000001_create_article_events_table.up.sql")
	articleID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	opened := models.ArticleEvent{Event: models.Event{
		ID: uuid.New().String(), Type: models.ArticleOpened,
		ArticleID: articleID, VisitorID: uuid.New().String(), OccurredAt: now.Add(-2 * time.Hour),
	}, ScoreDelta: 3}
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{opened}); err != nil {
		t.Fatalf("store event before migration: %v", err)
	}
	for _, migration := range []string{
		"000002_create_article_stats_minute.up.sql",
		"000003_create_article_stats_hour.up.sql",
		"000004_create_article_stats_day.up.sql",
	} {
		applyStatsMigration(t, ctx, repository, migration)
	}

	unliked := models.ArticleEvent{Event: models.Event{
		ID: uuid.New().String(), Type: models.ArticleUnliked,
		ArticleID: articleID, UserID: uuid.New().String(), OccurredAt: now,
	}, ScoreDelta: -6}
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{unliked}); err != nil {
		t.Fatalf("store event after migration: %v", err)
	}
	duplicate := unliked
	duplicate.Event.Title = "duplicate delivery"
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{duplicate}); err != nil {
		t.Fatalf("store duplicate event: %v", err)
	}
	created := models.ArticleEvent{Event: models.Event{
		ID: uuid.New().String(), Type: models.ArticleCreated,
		ArticleID: articleID, AuthorID: uuid.New().String(), OccurredAt: now, Title: "Article",
	}}
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{created}); err != nil {
		t.Fatalf("store article metadata event: %v", err)
	}

	for _, test := range []struct {
		table    string
		truncate func(time.Time) time.Time
	}{
		{"article_stats_minute", func(value time.Time) time.Time { return value.Truncate(time.Minute) }},
		{"article_stats_hour", func(value time.Time) time.Time { return value.Truncate(time.Hour) }},
		{"article_stats_day", func(value time.Time) time.Time { return value.Truncate(24 * time.Hour) }},
	} {
		t.Run(test.table, func(t *testing.T) {
			rows, err := repository.conn.Query(ctx, fmt.Sprintf(`SELECT bucket_start, event_type, score_delta, uniqExactMerge(event_ids)
				FROM %s WHERE article_id = ? GROUP BY bucket_start, event_type, score_delta`, test.table), articleID)
			if err != nil {
				t.Fatalf("query aggregates: %v", err)
			}
			defer rows.Close()
			got := make(map[models.EventType]struct {
				bucket time.Time
				score  int16
				count  uint64
			})
			for rows.Next() {
				var bucket time.Time
				var eventType string
				var score int16
				var count uint64
				if err := rows.Scan(&bucket, &eventType, &score, &count); err != nil {
					t.Fatalf("scan aggregates: %v", err)
				}
				got[models.EventType(eventType)] = struct {
					bucket time.Time
					score  int16
					count  uint64
				}{bucket, score, count}
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("read aggregates: %v", err)
			}
			if len(got) != 2 || got[models.ArticleOpened].count != 1 || got[models.ArticleOpened].score != 3 ||
				!got[models.ArticleOpened].bucket.Equal(test.truncate(opened.Event.OccurredAt)) ||
				got[models.ArticleUnliked].count != 1 || got[models.ArticleUnliked].score != -6 ||
				!got[models.ArticleUnliked].bucket.Equal(test.truncate(unliked.Event.OccurredAt)) {
				t.Fatalf("aggregates = %+v", got)
			}
		})
	}
}

func applyStatsMigration(t *testing.T, ctx context.Context, repository *ClickHouse, name string) {
	t.Helper()
	data, err := os.ReadFile("../../migrations/" + name)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	for _, statement := range strings.Split(string(data), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if err := repository.conn.Exec(ctx, statement); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}
