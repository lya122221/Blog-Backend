package repositories

import (
	"blog/internal/models"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"uuid"
)

func (s *Storage) insertArticleEvent(ctx context.Context, tx *sql.Tx, event models.AnalyticsEvent) error {
	event.Version = models.AnalyticsSchemaVersion
	event.ID = uuid.NewV4().String()
	event.OccurredAt = time.Now().UTC()
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal article event: %w", err)
	}
	return s.InsertOutboxEvent(ctx, tx, models.OutboxEvent{
		ID:        event.ID,
		Type:      event.Type,
		ArticleID: event.ArticleID,
		Payload:   payload,
	})
}
