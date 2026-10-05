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

type producerStub struct {
	batches [][]models.Event
	err     error
}

type limiterStub struct {
	keys map[string]string
}

func (stub *limiterStub) Acquire(_ context.Context, event models.Event) (bool, error) {
	if stub.keys == nil {
		stub.keys = make(map[string]string)
	}
	key := event.VisitorID + ":" + event.ArticleID + ":" + string(event.Type)
	if _, exists := stub.keys[key]; exists {
		return false, nil
	}
	stub.keys[key] = event.ID
	return true, nil
}

func (stub *limiterStub) Release(_ context.Context, event models.Event) error {
	key := event.VisitorID + ":" + event.ArticleID + ":" + string(event.Type)
	if stub.keys[key] == event.ID {
		delete(stub.keys, key)
	}
	return nil
}

func (stub *producerStub) Publish(_ context.Context, batch []models.Event) error {
	stub.batches = append(stub.batches, batch)
	return stub.err
}

var fixedNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func newTestService(stub *producerStub) *ViewsService {
	return &ViewsService{producer: stub, limiter: &limiterStub{}, now: func() time.Time { return fixedNow }}
}

func validView() models.ViewEvent {
	return models.ViewEvent{
		ID: uuid.New().String(), Type: models.ArticleOpened,
		ArticleID: uuid.New().String(), OccurredAt: fixedNow.Add(-time.Minute),
	}
}

func TestRecordViewsPublishesValidatedBatch(t *testing.T) {
	stub := &producerStub{}
	service := newTestService(stub)
	opened := validView()
	impression := validView()
	impression.Type = models.ArticleImpression
	visitorID := uuid.New().String()
	request := models.ViewRequest{Events: []models.ViewEvent{opened, impression}}
	if accepted, err := service.RecordViews(context.Background(), request, visitorID); err != nil || accepted != 2 {
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
			stub := &producerStub{}
			_, err := newTestService(stub).RecordViews(context.Background(), test.request, uuid.New().String())
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) || !strings.Contains(err.Error(), test.message) || len(stub.batches) != 0 {
				t.Fatalf("error = %v, published batches = %d", err, len(stub.batches))
			}
		})
	}
}

func TestRecordViewsPropagatesProducerFailure(t *testing.T) {
	brokerErr := errors.New("broker unavailable")
	stub := &producerStub{err: brokerErr}
	service := newTestService(stub)
	_, err := service.RecordViews(context.Background(), models.ViewRequest{Events: []models.ViewEvent{validView()}}, uuid.New().String())
	if !errors.Is(err, brokerErr) || len(stub.batches) != 1 {
		t.Fatalf("error = %v, published batches = %d", err, len(stub.batches))
	}
	if len(service.limiter.(*limiterStub).keys) != 0 {
		t.Fatal("view reservation was not released")
	}
}

func TestRecordViewsSuppressesRepeatedViewsPerVisitorArticleAndType(t *testing.T) {
	producer := &producerStub{}
	service := newTestService(producer)
	visitor := uuid.New().String()
	opened := validView()
	request := models.ViewRequest{Events: []models.ViewEvent{opened}}
	if accepted, err := service.RecordViews(context.Background(), request, visitor); err != nil || accepted != 1 {
		t.Fatalf("first view: accepted = %d, error = %v", accepted, err)
	}
	opened.ID = uuid.New().String()
	request.Events[0] = opened
	if accepted, err := service.RecordViews(context.Background(), request, visitor); err != nil || accepted != 0 {
		t.Fatalf("repeated view: accepted = %d, error = %v", accepted, err)
	}
	if len(producer.batches) != 1 {
		t.Fatalf("published batches = %d", len(producer.batches))
	}
	request.Events[0].Type = models.ArticleImpression
	if accepted, err := service.RecordViews(context.Background(), request, visitor); err != nil || accepted != 1 {
		t.Fatalf("other type: accepted = %d, error = %v", accepted, err)
	}
	request.Events[0].ID = uuid.New().String()
	if accepted, err := service.RecordViews(context.Background(), request, uuid.New().String()); err != nil || accepted != 1 {
		t.Fatalf("other visitor: accepted = %d, error = %v", accepted, err)
	}
	request.Events[0].ID = uuid.New().String()
	request.Events[0].ArticleID = uuid.New().String()
	if accepted, err := service.RecordViews(context.Background(), request, visitor); err != nil || accepted != 1 {
		t.Fatalf("other article: accepted = %d, error = %v", accepted, err)
	}
}

func TestRecordViewsReleasesReservationsOnLimiterFailure(t *testing.T) {
	producer := &producerStub{}
	service := newTestService(producer)
	limiter := service.limiter.(*limiterStub)
	first := validView()
	second := validView()
	service.limiter = &failingLimiter{limiterStub: limiter, failAt: 2}
	_, err := service.RecordViews(context.Background(), models.ViewRequest{Events: []models.ViewEvent{first, second}}, uuid.New().String())
	if err == nil || len(limiter.keys) != 0 || len(producer.batches) != 0 {
		t.Fatalf("error = %v, keys = %d, published batches = %d", err, len(limiter.keys), len(producer.batches))
	}
}

type failingLimiter struct {
	*limiterStub
	count  int
	failAt int
}

func (stub *failingLimiter) Acquire(ctx context.Context, event models.Event) (bool, error) {
	stub.count++
	if stub.count == stub.failAt {
		return false, errors.New("Redis unavailable")
	}
	return stub.limiterStub.Acquire(ctx, event)
}
