package services

import (
	"analytics/internal/models"
	"fmt"
	"math"
)

type InvalidEventError struct {
	err error
}

func (err *InvalidEventError) Error() string {
	return fmt.Sprintf("invalid event: %v", err.err)
}

func (err *InvalidEventError) Unwrap() error {
	return err.err
}

type EventsService struct {
	weights models.Weights
}

func NewEventsService(weights models.Weights) (*EventsService, error) {
	for _, weight := range []struct {
		name  string
		value int
	}{
		{"impression", weights.Impression},
		{"open", weights.Open},
		{"like", weights.Like},
		{"comment", weights.Comment},
	} {
		if weight.value < 0 || weight.value > math.MaxInt16 {
			return nil, fmt.Errorf("invalid %s weight %d: must fit ClickHouse Int16 and be non-negative", weight.name, weight.value)
		}
	}
	return &EventsService{weights: weights}, nil
}

func (service *EventsService) PrepareEvent(event models.Event) (models.ArticleEvent, error) {
	if err := event.Validate(); err != nil {
		return models.ArticleEvent{}, &InvalidEventError{err: err}
	}
	delta, err := service.weights.ScoreDelta(event.Type)
	if err != nil {
		return models.ArticleEvent{}, &InvalidEventError{err: err}
	}
	return models.ArticleEvent{Event: event, ScoreDelta: int16(delta)}, nil
}
