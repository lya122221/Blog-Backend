package services

import (
	"analytics/internal/models"
	"context"
	"errors"
	"fmt"
	"time"
)

const maxBatchSize = 100

type ViewsProducer interface {
	Publish(context.Context, []models.Event) error
}

type ViewsLimiter interface {
	Acquire(context.Context, models.Event) (bool, error)
	Release(context.Context, models.Event) error
}

type ViewsService struct {
	producer ViewsProducer
	limiter  ViewsLimiter
	now      func() time.Time
}

func NewViewsService(producer ViewsProducer, limiter ViewsLimiter) *ViewsService {
	return &ViewsService{producer: producer, limiter: limiter, now: time.Now}
}

type ValidationError struct {
	message string
}

func (err *ValidationError) Error() string {
	return err.message
}

func (service *ViewsService) RecordViews(ctx context.Context, request models.ViewRequest, visitorID string) (int, error) {
	if len(request.Events) == 0 || len(request.Events) > maxBatchSize {
		return 0, &ValidationError{message: "events must contain 1 to 100 items"}
	}

	now := service.now()
	batch := make([]models.Event, 0, len(request.Events))
	seenIDs := make(map[string]struct{}, len(request.Events))
	for _, item := range request.Events {
		if item.Type != models.ArticleImpression && item.Type != models.ArticleOpened {
			return 0, &ValidationError{message: "unsupported view type"}
		}
		if item.OccurredAt.Before(now.Add(-30*24*time.Hour)) || item.OccurredAt.After(now.Add(2*time.Minute)) {
			return 0, &ValidationError{message: "event time is outside accepted range"}
		}
		if _, duplicate := seenIDs[item.ID]; duplicate {
			return 0, &ValidationError{message: "duplicate event ID in batch"}
		}
		seenIDs[item.ID] = struct{}{}
		event := models.Event{
			Version:    models.SchemaVersion,
			ID:         item.ID,
			Type:       item.Type,
			ArticleID:  item.ArticleID,
			VisitorID:  visitorID,
			OccurredAt: item.OccurredAt,
		}
		if err := event.Validate(); err != nil {
			return 0, &ValidationError{message: "invalid event"}
		}
		batch = append(batch, event)
	}

	accepted := make([]models.Event, 0, len(batch))
	for _, event := range batch {
		reserved, err := service.limiter.Acquire(ctx, event)
		if err != nil {
			return 0, errors.Join(fmt.Errorf("check view limit: %w", err), service.release(ctx, accepted))
		}
		if reserved {
			accepted = append(accepted, event)
		}
	}
	if len(accepted) == 0 {
		return 0, nil
	}
	if err := service.producer.Publish(ctx, accepted); err != nil {
		return 0, errors.Join(fmt.Errorf("publish view events: %w", err), service.release(ctx, accepted))
	}
	return len(accepted), nil
}

func (service *ViewsService) release(ctx context.Context, events []models.Event) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	var releaseErr error
	for _, event := range events {
		releaseErr = errors.Join(releaseErr, service.limiter.Release(cleanupCtx, event))
	}
	return releaseErr
}
