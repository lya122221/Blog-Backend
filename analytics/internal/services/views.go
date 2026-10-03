package services

import (
	"analytics/internal/models"
	"context"
	"fmt"
	"time"
)

const maxBatchSize = 100

type ViewsPublisher interface {
	Publish(context.Context, []models.Event) error
}

type ViewsService struct {
	publisher ViewsPublisher
	now       func() time.Time
}

func NewViewsService(publisher ViewsPublisher) *ViewsService {
	return &ViewsService{publisher: publisher, now: time.Now}
}

type ValidationError struct {
	message string
}

func (err *ValidationError) Error() string {
	return err.message
}

func (service *ViewsService) RecordViews(ctx context.Context, request models.ViewRequest, visitorID string) error {
	if len(request.Events) == 0 || len(request.Events) > maxBatchSize {
		return &ValidationError{message: "events must contain 1 to 100 items"}
	}

	now := service.now()
	batch := make([]models.Event, 0, len(request.Events))
	seenIDs := make(map[string]struct{}, len(request.Events))
	for _, item := range request.Events {
		if item.Type != models.ArticleImpression && item.Type != models.ArticleOpened {
			return &ValidationError{message: "unsupported view type"}
		}
		if item.OccurredAt.Before(now.Add(-30*24*time.Hour)) || item.OccurredAt.After(now.Add(2*time.Minute)) {
			return &ValidationError{message: "event time is outside accepted range"}
		}
		if _, duplicate := seenIDs[item.ID]; duplicate {
			return &ValidationError{message: "duplicate event ID in batch"}
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
			return &ValidationError{message: "invalid event"}
		}
		batch = append(batch, event)
	}
	if err := service.publisher.Publish(ctx, batch); err != nil {
		return fmt.Errorf("publish view events: %w", err)
	}
	return nil
}
