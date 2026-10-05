package repositories

import (
	"blog/internal/models"
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestOutboxPostgres(t *testing.T) {
	dsn := os.Getenv("BLOG_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set BLOG_TEST_POSTGRES_DSN to run the PostgreSQL integration test")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	schema := "outbox_test_" + strings.ReplaceAll(uuid.NewV4().String(), "-", "")
	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := conn.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	}()
	if _, err := conn.ExecContext(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/000010_create_outbox_events_table.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(migration), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("apply outbox migration: %v", err)
		}
	}
	legacyID := uuid.NewV4().String()
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO outbox_events (event_id, event_type, article_id, payload)
		VALUES ($1, 'article.created', $2, '{}')
	`, legacyID, uuid.NewV4().String()); err != nil {
		t.Fatal(err)
	}
	orderMigration, err := os.ReadFile("../../migrations/000011_order_outbox_events.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(orderMigration), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("apply ordered outbox migration: %v", err)
		}
	}
	var legacyPosition int64
	if err := conn.QueryRowContext(ctx, "SELECT position FROM outbox_events WHERE event_id = $1", legacyID).Scan(&legacyPosition); err != nil || legacyPosition < 1 {
		t.Fatalf("existing outbox event has position %d: %v", legacyPosition, err)
	}
	if _, err := conn.ExecContext(ctx, "DELETE FROM outbox_events WHERE event_id = $1", legacyID); err != nil {
		t.Fatal(err)
	}

	s := &Storage{db: db}
	event := models.OutboxEvent{
		ID:        uuid.NewV4().String(),
		Type:      "article.created",
		ArticleID: uuid.NewV4().String(),
		Payload:   []byte(`{"version":1,"type":"article.created"}`),
	}

	rolledBack, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertOutboxEvent(ctx, rolledBack, event); err != nil {
		t.Fatal(err)
	}
	if err := rolledBack.Rollback(); err != nil {
		t.Fatal(err)
	}

	committed, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertOutboxEvent(ctx, committed, event); err != nil {
		_ = committed.Rollback()
		t.Fatalf("rolled back outbox event remained in the database: %v", err)
	}
	if err := committed.Commit(); err != nil {
		t.Fatal(err)
	}

	secondConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer secondConn.Close()
	if _, err := secondConn.ExecContext(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	firstTx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = firstTx.Rollback() }()
	first, err := s.ReadPendingOutboxEvents(ctx, firstTx, 10)
	if err != nil || len(first) != 1 || first[0].ID != event.ID {
		t.Fatalf("first claim = %+v, %v", first, err)
	}
	secondTx, err := secondConn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = secondTx.Rollback() }()
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	second, err := s.ReadPendingOutboxEvents(readCtx, secondTx, 10)
	if err != nil || len(second) != 0 {
		t.Fatalf("locked event was read twice: %+v, %v", second, err)
	}
	publishedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := s.MarkOutboxEventPublished(ctx, firstTx, event.ID, publishedAt); err != nil {
		t.Fatal(err)
	}
	if err := firstTx.Commit(); err != nil {
		t.Fatal(err)
	}
	second, err = s.ReadPendingOutboxEvents(ctx, secondTx, 10)
	if err != nil || len(second) != 0 {
		t.Fatalf("published event is pending: %+v, %v", second, err)
	}
	if err := secondTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var storedPublishedAt sql.NullTime
	if err := conn.QueryRowContext(ctx, "SELECT published_at FROM outbox_events WHERE event_id = $1", event.ID).Scan(&storedPublishedAt); err != nil || !storedPublishedAt.Valid || !storedPublishedAt.Time.Equal(publishedAt) {
		t.Fatalf("published_at = %v, want %v, err = %v", storedPublishedAt, publishedAt, err)
	}
	secondEvent := event
	secondEvent.ID = uuid.NewV4().String()
	thirdEvent := event
	thirdEvent.ID = uuid.NewV4().String()
	otherArticle := event
	otherArticle.ID = uuid.NewV4().String()
	otherArticle.ArticleID = uuid.NewV4().String()
	insertTx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, pending := range []models.OutboxEvent{secondEvent, thirdEvent, otherArticle} {
		if err := s.InsertOutboxEvent(ctx, insertTx, pending); err != nil {
			_ = insertTx.Rollback()
			t.Fatal(err)
		}
	}
	if err := insertTx.Commit(); err != nil {
		t.Fatal(err)
	}
	orderedTx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = orderedTx.Rollback() }()
	ordered, err := s.ReadPendingOutboxEvents(ctx, orderedTx, 10)
	if err != nil || len(ordered) != 2 || ordered[0].ID != secondEvent.ID || ordered[1].ID != otherArticle.ID {
		t.Fatalf("ordered pending events = %+v, err = %v", ordered, err)
	}
	concurrentTx, err := secondConn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = concurrentTx.Rollback() }()
	concurrent, err := s.ReadPendingOutboxEvents(ctx, concurrentTx, 10)
	if err != nil || len(concurrent) != 0 {
		t.Fatalf("concurrent worker bypassed pending article event: %+v, %v", concurrent, err)
	}
	for _, claimed := range ordered {
		if err := s.MarkOutboxEventPublished(ctx, orderedTx, claimed.ID, publishedAt); err != nil {
			t.Fatal(err)
		}
	}
	if err := orderedTx.Commit(); err != nil {
		t.Fatal(err)
	}
	concurrent, err = s.ReadPendingOutboxEvents(ctx, concurrentTx, 10)
	if err != nil || len(concurrent) != 1 || concurrent[0].ID != thirdEvent.ID {
		t.Fatalf("next article event = %+v, err = %v", concurrent, err)
	}
	if err := s.MarkOutboxEventPublished(ctx, concurrentTx, thirdEvent.ID, publishedAt); err != nil {
		t.Fatal(err)
	}
	if err := concurrentTx.Commit(); err != nil {
		t.Fatal(err)
	}
	cleanupDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupDB.Close()
	cleanupDB.SetMaxOpenConns(1)
	if _, err := cleanupDB.ExecContext(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	cleanupStore := &Storage{db: cleanupDB}
	deleted, err := cleanupStore.DeletePublishedOutboxEventsBefore(ctx, publishedAt.Add(time.Second), 2)
	if err != nil || deleted != 2 {
		t.Fatalf("first cleanup deleted %d: %v", deleted, err)
	}
	deleted, err = cleanupStore.DeletePublishedOutboxEventsBefore(ctx, publishedAt.Add(time.Second), 10)
	if err != nil || deleted != 2 {
		t.Fatalf("second cleanup deleted %d: %v", deleted, err)
	}
	orderDown, err := os.ReadFile("../../migrations/000011_order_outbox_events.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(orderDown), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("revert ordered outbox migration: %v", err)
		}
	}
	downMigration, err := os.ReadFile("../../migrations/000010_create_outbox_events_table.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, string(downMigration)); err != nil {
		t.Fatalf("revert outbox migration: %v", err)
	}
	var tableName sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT to_regclass('outbox_events')").Scan(&tableName); err != nil || tableName.Valid {
		t.Fatalf("outbox table remains after down migration: %v, %v", tableName, err)
	}
}
