package repositories

import (
	"analytics/internal/config"
	"analytics/internal/models"
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const insertArticleEvents = `INSERT INTO article_events
	(event_id, event_type, article_id, author_id, user_id, visitor_id, occurred_at, score_delta, title, tags)`

type ClickHouse struct {
	conn driver.Conn
}

func NewClickHouse(ctx context.Context, settings config.ClickHouseConfig) (*ClickHouse, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{settings.Addr},
		Auth: clickhouse.Auth{
			Database: settings.Database,
			Username: settings.User,
			Password: settings.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open ClickHouse connection: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping ClickHouse: %w", err)
	}
	return &ClickHouse{conn: conn}, nil
}

func (repository *ClickHouse) StoreBatch(ctx context.Context, events []models.ArticleEvent) error {
	if len(events) == 0 {
		return nil
	}
	batch, err := repository.conn.PrepareBatch(ctx, insertArticleEvents)
	if err != nil {
		return fmt.Errorf("prepare ClickHouse batch: %w", err)
	}
	defer func() { _ = batch.Close() }()

	for _, item := range events {
		event := item.Event
		if err := batch.Append(
			event.ID,
			string(event.Type),
			event.ArticleID,
			nullableID(event.AuthorID),
			nullableID(event.UserID),
			nullableID(event.VisitorID),
			event.OccurredAt,
			item.ScoreDelta,
			event.Title,
			event.Tags,
		); err != nil {
			return fmt.Errorf("append event %s to ClickHouse batch: %w", event.ID, err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send ClickHouse batch: %w", err)
	}
	return nil
}

func (repository *ClickHouse) Close() error {
	return repository.conn.Close()
}

func nullableID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
