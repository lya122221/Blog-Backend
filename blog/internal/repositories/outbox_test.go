package repositories

import (
	"blog/internal/models"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestOutboxRepository(t *testing.T) {
	id := "629cf80d-0b69-4b32-854d-a5ac7f886779"
	articleID := "1cc548d4-0651-465d-9342-a10e2a507c90"
	payload := []byte(`{"event_id":"629cf80d-0b69-4b32-854d-a5ac7f886779"}`)
	createdAt := time.Now()
	publishedAt := createdAt.Add(time.Minute)
	c := &testConn{}
	c.exec = func(q string, args []driver.NamedValue) (driver.Result, error) {
		switch {
		case has(q, "INSERT INTO outbox_events"):
			if len(args) != 4 || args[0].Value != id || args[1].Value != "article.created" || args[2].Value != articleID || args[3].Value != string(payload) {
				return nil, fmt.Errorf("insert args = %v", args)
			}
			return driver.RowsAffected(1), nil
		case has(q, "UPDATE outbox_events"):
			if len(args) != 2 || args[0].Value != id || args[1].Value != publishedAt || !has(q, "published_at = $2") {
				return nil, fmt.Errorf("publish query = %s, args = %v", q, args)
			}
			return driver.RowsAffected(1), nil
		default:
			return nil, fmt.Errorf("unexpected exec: %s", q)
		}
	}
	c.query = func(q string, args []driver.NamedValue) (driver.Rows, error) {
		if !has(q, "FOR UPDATE SKIP LOCKED") || !has(q, "WHERE published_at IS NULL") || len(args) != 1 || fmt.Sprint(args[0].Value) != "10" {
			return nil, fmt.Errorf("pending query = %s, args = %v", q, args)
		}
		return rows(
			[]string{"event_id", "event_type", "article_id", "payload", "created_at"},
			[]driver.Value{id, "article.created", articleID, payload, createdAt},
		), nil
	}
	s := &Storage{db: openTestDB(t, c)}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	event := models.OutboxEvent{ID: id, Type: "article.created", ArticleID: articleID, Payload: payload}
	if err := s.InsertOutboxEvent(context.Background(), tx, event); err != nil {
		t.Fatalf("InsertOutboxEvent: %v", err)
	}
	got, err := s.ReadPendingOutboxEvents(context.Background(), tx, 10)
	if err != nil || len(got) != 1 || got[0].ID != id || string(got[0].Payload) != string(payload) || !got[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("ReadPendingOutboxEvents = %+v, %v", got, err)
	}
	if err := s.MarkOutboxEventPublished(context.Background(), tx, id, publishedAt); err != nil {
		t.Fatalf("MarkOutboxEventPublished: %v", err)
	}
}

func TestOutboxRepositoryValidation(t *testing.T) {
	s := &Storage{}
	if err := s.InsertOutboxEvent(context.Background(), nil, models.OutboxEvent{}); err == nil {
		t.Fatal("expected nil transaction error")
	}
	if _, err := s.ReadPendingOutboxEvents(context.Background(), nil, 1); err == nil {
		t.Fatal("expected nil transaction error")
	}
	if err := s.MarkOutboxEventPublished(context.Background(), nil, "id", time.Now()); err == nil {
		t.Fatal("expected nil transaction error")
	}

	c := &testConn{}
	s.db = openTestDB(t, c)
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := s.MarkOutboxEventPublished(context.Background(), tx, "id", time.Time{}); err == nil {
		t.Fatal("expected missing publication time error")
	}
	if err := s.InsertOutboxEvent(context.Background(), tx, models.OutboxEvent{Payload: []byte("{")}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if _, err := s.ReadPendingOutboxEvents(context.Background(), tx, 0); err == nil {
		t.Fatal("expected invalid limit error")
	}
}

func TestOutboxRepositoryErrors(t *testing.T) {
	c := &testConn{}
	s := &Storage{db: openTestDB(t, c)}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	c.exec = func(string, []driver.NamedValue) (driver.Result, error) {
		return nil, errors.New("database unavailable")
	}
	if err := s.InsertOutboxEvent(context.Background(), tx, models.OutboxEvent{Payload: []byte(`{}`)}); err == nil {
		t.Fatal("expected insert error")
	}
	if err := s.MarkOutboxEventPublished(context.Background(), tx, "id", time.Now()); err == nil {
		t.Fatal("expected update error")
	}
	c.exec = func(string, []driver.NamedValue) (driver.Result, error) {
		return driver.RowsAffected(0), nil
	}
	if err := s.MarkOutboxEventPublished(context.Background(), tx, "id", time.Now()); err == nil {
		t.Fatal("expected missing pending event error")
	}
	c.query = func(string, []driver.NamedValue) (driver.Rows, error) {
		return nil, errors.New("database unavailable")
	}
	if _, err := s.ReadPendingOutboxEvents(context.Background(), tx, 1); err == nil {
		t.Fatal("expected read error")
	}
}
