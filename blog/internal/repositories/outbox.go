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
		SELECT event_id, event_type, article_id, payload, created_at
		FROM outbox_events	
		WHERE published_at IS NULL
		ORDER BY created_at, event_id
		LIMIT $1
		FOR UPDATE SKIP LOCKED
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
