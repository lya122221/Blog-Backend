package services

import (
	"analytics/internal/models"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"
)

func validAnalyticsEvent(eventType models.EventType) models.Event {
	return models.Event{
		Version:    models.SchemaVersion,
		ID:         uuid.New().String(),
		Type:       eventType,
		ArticleID:  uuid.New().String(),
		AuthorID:   uuid.New().String(),
		UserID:     uuid.New().String(),
		VisitorID:  uuid.New().String(),
		OccurredAt: time.Date(2026, time.October, 4, 12, 0, 0, 123456000, time.UTC),
		Title:      "Article title",
		Tags:       []string{"go", "analytics"},
	}
}

func TestPrepareEventCalculatesScores(t *testing.T) {
	service, err := NewEventsService(models.DefaultWeights())
	if err != nil {
		t.Fatalf("NewEventsService: %v", err)
	}
	for _, test := range []struct {
		eventType models.EventType
		wantDelta int16
	}{
		{models.ArticleImpression, 1},
		{models.ArticleOpened, 3},
		{models.ArticleLiked, 6},
		{models.ArticleUnliked, -6},
		{models.CommentCreated, 6},
		{models.ArticleCreated, 0},
		{models.ArticleUpdated, 0},
		{models.ArticleDeleted, 0},
	} {
		t.Run(string(test.eventType), func(t *testing.T) {
			event := validAnalyticsEvent(test.eventType)
			prepared, err := service.PrepareEvent(event)
			if err != nil {
				t.Fatalf("PrepareEvent: %v", err)
			}
			if prepared.ScoreDelta != test.wantDelta || !reflect.DeepEqual(prepared.Event, event) {
				t.Fatalf("prepared event = %+v, want score %d and original event", prepared, test.wantDelta)
			}
		})
	}
}

func TestPrepareEventUsesCustomWeights(t *testing.T) {
	service, err := NewEventsService(models.Weights{Impression: 2, Open: 4, Like: 9, Comment: 5})
	if err != nil {
		t.Fatalf("NewEventsService: %v", err)
	}
	for _, test := range []struct {
		eventType models.EventType
		wantDelta int16
	}{
		{models.ArticleImpression, 2},
		{models.ArticleOpened, 4},
		{models.ArticleLiked, 9},
		{models.ArticleUnliked, -9},
		{models.CommentCreated, 5},
	} {
		prepared, err := service.PrepareEvent(validAnalyticsEvent(test.eventType))
		if err != nil || prepared.ScoreDelta != test.wantDelta {
			t.Fatalf("PrepareEvent(%q) = %+v, %v; want delta %d", test.eventType, prepared, err, test.wantDelta)
		}
	}
}

func TestPrepareEventRejectsInvalidEvents(t *testing.T) {
	service, err := NewEventsService(models.DefaultWeights())
	if err != nil {
		t.Fatalf("NewEventsService: %v", err)
	}
	for _, test := range []struct {
		name    string
		change  func(*models.Event)
		message string
	}{
		{"version", func(event *models.Event) { event.Version = 2 }, "version"},
		{"event ID", func(event *models.Event) { event.ID = "bad" }, "event ID"},
		{"optional UUID", func(event *models.Event) { event.AuthorID = "bad" }, "author ID"},
		{"unknown type", func(event *models.Event) { event.Type = "unknown" }, "event type"},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := validAnalyticsEvent(models.ArticleOpened)
			test.change(&event)
			prepared, err := service.PrepareEvent(event)
			var invalid *InvalidEventError
			if !errors.As(err, &invalid) || errors.Unwrap(err) == nil || !strings.Contains(err.Error(), test.message) || !reflect.DeepEqual(prepared, models.ArticleEvent{}) {
				t.Fatalf("PrepareEvent = %+v, %v", prepared, err)
			}
		})
	}
}

func TestNewEventsServiceRejectsWeightsOutsideClickHouseRange(t *testing.T) {
	for _, weights := range []models.Weights{
		{Impression: -1, Open: 3, Like: 6, Comment: 6},
		{Impression: 1, Open: 3, Like: math.MaxInt16 + 1, Comment: 6},
	} {
		if _, err := NewEventsService(weights); err == nil {
			t.Fatalf("NewEventsService accepted weights %+v", weights)
		}
	}
}
