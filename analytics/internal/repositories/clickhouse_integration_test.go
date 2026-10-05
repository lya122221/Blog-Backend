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

func TestClickHouseStoreBatchIntegration(t *testing.T) {
	addr := os.Getenv("ANALYTICS_TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		t.Skip("set ANALYTICS_TEST_CLICKHOUSE_ADDR to run ClickHouse integration test")
	}
	settings := config.ClickHouseConfig{
		Addr:     addr,
		Database: os.Getenv("ANALYTICS_TEST_CLICKHOUSE_DATABASE"),
		User:     os.Getenv("ANALYTICS_TEST_CLICKHOUSE_USER"),
		Password: os.Getenv("ANALYTICS_TEST_CLICKHOUSE_PASSWORD"),
	}
	if settings.Database == "" || settings.User == "" {
		t.Fatal("set ANALYTICS_TEST_CLICKHOUSE_DATABASE and ANALYTICS_TEST_CLICKHOUSE_USER")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := NewClickHouse(ctx, settings)
	if err != nil {
		t.Fatalf("connect to ClickHouse: %v", err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Errorf("close ClickHouse connection: %v", err)
		}
	})

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
	defer func() {
		if err := repository.Close(); err != nil {
			t.Errorf("close test repository: %v", err)
		}
	}()
	migration, err := os.ReadFile("../../migrations/000001_create_article_events_table.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if err := repository.conn.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply event migration: %v", err)
	}

	occurredAt := time.Now().UTC().Truncate(time.Microsecond)
	first := models.ArticleEvent{Event: models.Event{
		ID:         uuid.New().String(),
		Type:       models.ArticleOpened,
		ArticleID:  uuid.New().String(),
		AuthorID:   uuid.New().String(),
		UserID:     uuid.New().String(),
		VisitorID:  uuid.New().String(),
		OccurredAt: occurredAt,
		Title:      "Article title",
		Tags:       []string{"go", "analytics"},
	}, ScoreDelta: 3}
	second := models.ArticleEvent{Event: models.Event{
		ID:         uuid.New().String(),
		Type:       models.ArticleImpression,
		ArticleID:  uuid.New().String(),
		VisitorID:  uuid.New().String(),
		OccurredAt: occurredAt,
	}, ScoreDelta: 1}
	if err := repository.StoreBatch(ctx, []models.ArticleEvent{first, second}); err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}

	rows, err := repository.conn.Query(ctx, `SELECT toString(event_id), event_type, toString(article_id),
		ifNull(toString(author_id), ''), ifNull(toString(user_id), ''), ifNull(toString(visitor_id), ''),
		occurred_at, score_delta, title, tags, ingested_at FROM article_events FINAL`)
	if err != nil {
		t.Fatalf("query stored events: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close event rows: %v", err)
		}
	}()
	stored := make(map[string]models.ArticleEvent)
	for rows.Next() {
		var event models.ArticleEvent
		var eventType string
		var ingestedAt time.Time
		if err := rows.Scan(&event.Event.ID, &eventType, &event.Event.ArticleID,
			&event.Event.AuthorID, &event.Event.UserID, &event.Event.VisitorID,
			&event.Event.OccurredAt, &event.ScoreDelta, &event.Event.Title, &event.Event.Tags, &ingestedAt); err != nil {
			t.Fatalf("scan stored event: %v", err)
		}
		if ingestedAt.IsZero() {
			t.Fatal("ingested_at was not set")
		}
		event.Event.Type = models.EventType(eventType)
		stored[event.Event.ID] = event
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read stored events: %v", err)
	}
	for _, want := range []models.ArticleEvent{first, second} {
		got, ok := stored[want.Event.ID]
		if !ok {
			t.Fatalf("event %s was not stored", want.Event.ID)
		}
		if !got.Event.OccurredAt.Equal(want.Event.OccurredAt) {
			t.Fatalf("event time = %v, want %v", got.Event.OccurredAt, want.Event.OccurredAt)
		}
		got.Event.OccurredAt = want.Event.OccurredAt
		if len(got.Event.Tags) == 0 && len(want.Event.Tags) == 0 {
			got.Event.Tags = want.Event.Tags
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("stored event = %+v, want %+v", got, want)
		}
	}
	if len(stored) != 2 {
		t.Fatalf("stored %d events, want 2", len(stored))
	}
}
