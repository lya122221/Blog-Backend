package repositories

import (
	"blog/internal/models"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (s *Storage) InsertOutboxEvent(ctx context.Context, tx *sql.Tx, event models.OutboxEvent) error {
	if tx == nil {
		return errors.New("outbox transaction is nil")
	}
	if !json.Valid(event.Payload) {
		return errors.New("invalid outbox event payload")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_events (event_id, event_type, article_id, payload)
		VALUES ($1, $2, $3, $4::jsonb)
	`, event.ID, event.Type, event.ArticleID, string(event.Payload))
	if err != nil {
		return fmt.Errorf("insert outbox event %s: %w", event.ID, err)
	}
	return nil
}

func (s *Storage) ReadPendingOutboxEvents(ctx context.Context, tx *sql.Tx, limit int) ([]models.OutboxEvent, error) {
	if tx == nil {
		return nil, errors.New("outbox transaction is nil")
	}
	if limit < 1 {
		return nil, errors.New("outbox limit must be positive")
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT e.event_id, e.event_type, e.article_id, e.payload, e.created_at
		FROM outbox_events AS e
		WHERE e.published_at IS NULL
		  AND NOT EXISTS (
			SELECT 1
			FROM outbox_events AS earlier
			WHERE earlier.article_id = e.article_id
			  AND earlier.published_at IS NULL
			  AND earlier.position < e.position
		  )
		ORDER BY e.position
		LIMIT $1
		FOR UPDATE OF e SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("read pending outbox events: %w", err)
	}
	defer rows.Close()

	var events []models.OutboxEvent
	for rows.Next() {
		var event models.OutboxEvent
		if err := rows.Scan(&event.ID, &event.Type, &event.ArticleID, &event.Payload, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan pending outbox event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending outbox events: %w", err)
	}
	return events, nil
}

func (s *Storage) BeginOutboxTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin outbox transaction: %w", err)
	}
	return tx, nil
}

func (s *Storage) MarkOutboxEventPublished(ctx context.Context, tx *sql.Tx, eventID string, publishedAt time.Time) error {
	if tx == nil {
		return errors.New("outbox transaction is nil")
	}
	if publishedAt.IsZero() {
		return errors.New("outbox publication time is missing")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE outbox_events
		SET published_at = $2
		WHERE event_id = $1 AND published_at IS NULL
	`, eventID, publishedAt)
	if err != nil {
		return fmt.Errorf("mark outbox event %s published: %w", eventID, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check published outbox event %s: %w", eventID, err)
	}
	if updated != 1 {
		return fmt.Errorf("outbox event %s is not pending", eventID)
	}
	return nil
}

func (s *Storage) DeletePublishedOutboxEventsBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if before.IsZero() || limit < 1 {
		return 0, errors.New("invalid outbox cleanup boundary or limit")
	}
	result, err := s.db.ExecContext(ctx, `
		WITH expired AS (
			SELECT event_id FROM outbox_events
			WHERE published_at < $1
			ORDER BY published_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM outbox_events
		WHERE event_id IN (SELECT event_id FROM expired)
	`, before, limit)
	if err != nil {
		return 0, fmt.Errorf("delete published outbox events: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted outbox events: %w", err)
	}
	return deleted, nil
}
