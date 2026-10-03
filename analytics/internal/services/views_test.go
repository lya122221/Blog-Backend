package services

import (
	"analytics/internal/models"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"
)

type publisherStub struct {
	batches [][]models.Event
	err     error
}

func (stub *publisherStub) Publish(_ context.Context, batch []models.Event) error {
	stub.batches = append(stub.batches, batch)
	return stub.err
}

var fixedNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func newTestService(stub *publisherStub) *ViewsService {
	return &ViewsService{publisher: stub, now: func() time.Time { return fixedNow }}
}

func validView() models.ViewEvent {
	return models.ViewEvent{
		ID: uuid.New().String(), Type: models.ArticleOpened,
		ArticleID: uuid.New().String(), OccurredAt: fixedNow.Add(-time.Minute),
	}
}

func TestRecordViewsPublishesValidatedBatch(t *testing.T) {
	stub := &publisherStub{}
	service := newTestService(stub)
	opened := validView()
	impression := validView()
	impression.Type = models.ArticleImpression
	visitorID := uuid.New().String()
	request := models.ViewRequest{Events: []models.ViewEvent{opened, impression}}
	if err := service.RecordViews(context.Background(), request, visitorID); err != nil {
		t.Fatalf("RecordViews: %v", err)
	}
	if len(stub.batches) != 1 || len(stub.batches[0]) != 2 {
		t.Fatalf("published batches = %+v", stub.batches)
	}
	for index, event := range stub.batches[0] {
		item := request.Events[index]
		if event.Version != models.SchemaVersion || event.ID != item.ID || event.Type != item.Type || event.ArticleID != item.ArticleID || event.VisitorID != visitorID || !event.OccurredAt.Equal(item.OccurredAt) {
			t.Fatalf("published event = %+v, request = %+v", event, item)
		}
	}
}

func TestRecordViewsRejectsInvalidBatchBeforePublishing(t *testing.T) {
	duplicate := validView()
	invalidID := validView()
	invalidID.ID = "bad"
	unsupported := validView()
	unsupported.Type = models.ArticleLiked
	tooOld := validView()
	tooOld.OccurredAt = fixedNow.Add(-31 * 24 * time.Hour)
	tooNew := validView()
	tooNew.OccurredAt = fixedNow.Add(3 * time.Minute)

	cases := map[string]struct {
		request models.ViewRequest
		message string
	}{
		"empty batch":      {request: models.ViewRequest{}, message: "1 to 100"},
		"oversized batch":  {request: models.ViewRequest{Events: make([]models.ViewEvent, maxBatchSize+1)}, message: "1 to 100"},
		"duplicate IDs":    {request: models.ViewRequest{Events: []models.ViewEvent{duplicate, duplicate}}, message: "duplicate event ID"},
		"invalid ID":       {request: models.ViewRequest{Events: []models.ViewEvent{invalidID}}, message: "invalid event"},
		"unsupported type": {request: models.ViewRequest{Events: []models.ViewEvent{unsupported}}, message: "unsupported view type"},
		"old event":        {request: models.ViewRequest{Events: []models.ViewEvent{tooOld}}, message: "event time"},
		"future event":     {request: models.ViewRequest{Events: []models.ViewEvent{tooNew}}, message: "event time"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &publisherStub{}
			err := newTestService(stub).RecordViews(context.Background(), test.request, uuid.New().String())
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) || !strings.Contains(err.Error(), test.message) || len(stub.batches) != 0 {
				t.Fatalf("error = %v, published batches = %d", err, len(stub.batches))
			}
		})
	}
}

func TestRecordViewsPropagatesPublisherFailure(t *testing.T) {
	brokerErr := errors.New("broker unavailable")
	stub := &publisherStub{err: brokerErr}
	err := newTestService(stub).RecordViews(context.Background(), models.ViewRequest{Events: []models.ViewEvent{validView()}}, uuid.New().String())
	if !errors.Is(err, brokerErr) || len(stub.batches) != 1 {
		t.Fatalf("error = %v, published batches = %d", err, len(stub.batches))
	}
}
