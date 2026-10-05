package workers

import (
	"blog/internal/config"
	"blog/internal/models"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const cleanupBatchSize = 1000

type OutboxRepository interface {
	BeginOutboxTx(context.Context) (*sql.Tx, error)
	ReadPendingOutboxEvents(context.Context, *sql.Tx, int) ([]models.OutboxEvent, error)
	MarkOutboxEventPublished(context.Context, *sql.Tx, string, time.Time) error
	DeletePublishedOutboxEventsBefore(context.Context, time.Time, int) (int64, error)
}

type OutboxPublisher interface {
	Publish(context.Context, models.OutboxEvent) error
}

type OutboxWorker struct {
	repo      OutboxRepository
	publisher OutboxPublisher
	logger    *slog.Logger
	config    config.Outbox
	now       func() time.Time
}

func NewOutboxWorker(repo OutboxRepository, publisher OutboxPublisher, logger *slog.Logger, cfg config.Outbox) *OutboxWorker {
	return &OutboxWorker{repo: repo, publisher: publisher, logger: logger, config: cfg, now: time.Now}
}

func (worker *OutboxWorker) Run(ctx context.Context) error {
	pollTimer := time.NewTimer(0)
	defer pollTimer.Stop()
	cleanupTicker := time.NewTicker(worker.config.CleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-cleanupTicker.C:
			before := worker.now().Add(-worker.config.Retention)
			deleted, err := worker.repo.DeletePublishedOutboxEventsBefore(ctx, before, cleanupBatchSize)
			if err != nil && ctx.Err() == nil {
				worker.logger.Error("failed to clean published outbox events", "error", err)
			} else if deleted > 0 {
				worker.logger.Info("cleaned published outbox events", "count", deleted)
			}
		case <-pollTimer.C:
			count, err := worker.publishBatch(ctx)
			if ctx.Err() != nil {
				return nil
			}
			interval := worker.config.PollInterval
			if err != nil {
				worker.logger.Error("failed to publish outbox events", "error", err)
				interval = worker.config.RetryInterval
			} else if count > 0 {
				interval = 0
			}
			pollTimer.Reset(interval)
		}
	}
}

func (worker *OutboxWorker) publishBatch(ctx context.Context) (int, error) {
	tx, err := worker.repo.BeginOutboxTx(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	events, err := worker.repo.ReadPendingOutboxEvents(ctx, tx, worker.config.BatchSize)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	for _, event := range events {
		if err := worker.publisher.Publish(ctx, event); err != nil {
			return 0, fmt.Errorf("publish outbox event %s: %w", event.ID, err)
		}
		if err := worker.repo.MarkOutboxEventPublished(ctx, tx, event.ID, worker.now().UTC()); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit outbox batch: %w", err)
	}
	return len(events), nil
}
