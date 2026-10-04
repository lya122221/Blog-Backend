package consumer

import (
	"analytics/internal/models"
	"analytics/internal/services"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/twmb/franz-go/pkg/kgo"
)

type clientStub struct {
	polls            []func(context.Context) kgo.Fetches
	maxPollRecords   []int
	committed        [][]*kgo.Record
	commitErr        error
	onCommit         func()
	allowedRebalance int
	closed           bool
}

func (client *clientStub) PollRecords(ctx context.Context, maxRecords int) kgo.Fetches {
	client.maxPollRecords = append(client.maxPollRecords, maxRecords)
	if len(client.polls) == 0 {
		return kgo.NewErrFetch(errors.New("unexpected poll"))
	}
	poll := client.polls[0]
	client.polls = client.polls[1:]
	return poll(ctx)
}

func (client *clientStub) CommitRecords(_ context.Context, records ...*kgo.Record) error {
	client.committed = append(client.committed, append([]*kgo.Record(nil), records...))
	if client.onCommit != nil {
		client.onCommit()
	}
	return client.commitErr
}

func (client *clientStub) AllowRebalance() {
	client.allowedRebalance++
}

func (client *clientStub) Close() {
	client.closed = true
}

type storeStub struct {
	batches      [][]models.ArticleEvent
	err          error
	contextError error
	onStore      func()
}

func (store *storeStub) StoreBatch(ctx context.Context, batch []models.ArticleEvent) error {
	store.contextError = ctx.Err()
	store.batches = append(store.batches, append([]models.ArticleEvent(nil), batch...))
	if store.onStore != nil {
		store.onStore()
	}
	return store.err
}

func testConsumer(t *testing.T, client *clientStub, store *storeStub, batchSize int, flushInterval time.Duration) *Consumer {
	t.Helper()
	preparer, err := services.NewEventsService(models.DefaultWeights())
	if err != nil {
		t.Fatalf("NewEventsService: %v", err)
	}
	return &Consumer{client: client, preparer: preparer, store: store, batchSize: batchSize, flushInterval: flushInterval}
}

func testRecord(t *testing.T, offset int64) *kgo.Record {
	t.Helper()
	event := models.Event{
		Version:    models.SchemaVersion,
		ID:         uuid.New().String(),
		Type:       models.ArticleOpened,
		ArticleID:  uuid.New().String(),
		VisitorID:  uuid.New().String(),
		OccurredAt: time.Now().UTC(),
	}
	value, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}
	return &kgo.Record{Topic: "article-events", Partition: 0, Offset: offset, Value: value}
}

func fetched(records ...*kgo.Record) kgo.Fetches {
	return kgo.Fetches{{Topics: []kgo.FetchTopic{{
		Topic:      "article-events",
		Partitions: []kgo.FetchPartition{{Partition: 0, Records: records}},
	}}}}
}

func TestRunCommitsAfterFullBatchIsStored(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, second := testRecord(t, 10), testRecord(t, 11)
	var calls []string
	client := &clientStub{polls: []func(context.Context) kgo.Fetches{
		func(context.Context) kgo.Fetches { return fetched(first, second) },
	}, onCommit: func() {
		calls = append(calls, "commit")
		cancel()
	}}
	store := &storeStub{onStore: func() { calls = append(calls, "store") }}
	consumer := testConsumer(t, client, store, 2, time.Second)
	if err := consumer.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.batches) != 1 || len(store.batches[0]) != 2 || store.batches[0][0].ScoreDelta != 3 || store.batches[0][1].ScoreDelta != 3 {
		t.Fatalf("stored batches = %+v", store.batches)
	}
	if len(client.committed) != 1 || len(client.committed[0]) != 2 || client.committed[0][0] != first || client.committed[0][1] != second {
		t.Fatalf("committed records = %+v", client.committed)
	}
	if len(client.maxPollRecords) != 1 || client.maxPollRecords[0] != 2 || client.allowedRebalance == 0 {
		t.Fatalf("unexpected poll/rebalance state: limits=%v, allowed=%d", client.maxPollRecords, client.allowedRebalance)
	}
	if len(calls) != 2 || calls[0] != "store" || calls[1] != "commit" {
		t.Fatalf("store/commit order = %v", calls)
	}
}

func TestRunFlushesPartialBatchAfterInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &clientStub{polls: []func(context.Context) kgo.Fetches{
		func(context.Context) kgo.Fetches { return fetched(testRecord(t, 1)) },
		func(ctx context.Context) kgo.Fetches {
			<-ctx.Done()
			return kgo.NewErrFetch(ctx.Err())
		},
	}, onCommit: cancel}
	store := &storeStub{}
	consumer := testConsumer(t, client, store, 10, 10*time.Millisecond)
	if err := consumer.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.batches) != 1 || len(store.batches[0]) != 1 || len(client.committed) != 1 || len(client.committed[0]) != 1 {
		t.Fatalf("stored=%d, committed=%d", len(store.batches), len(client.committed))
	}
	if len(client.maxPollRecords) != 2 || client.maxPollRecords[1] != 9 {
		t.Fatalf("poll limits = %v", client.maxPollRecords)
	}
}

func TestRunFlushesPendingEventsOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &clientStub{polls: []func(context.Context) kgo.Fetches{
		func(context.Context) kgo.Fetches { return fetched(testRecord(t, 1)) },
		func(context.Context) kgo.Fetches {
			cancel()
			return kgo.NewErrFetch(context.Canceled)
		},
	}}
	store := &storeStub{}
	consumer := testConsumer(t, client, store, 10, time.Second)
	if err := consumer.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.batches) != 1 || len(client.committed) != 1 || store.contextError != nil {
		t.Fatalf("shutdown stored=%d, committed=%d, context error=%v", len(store.batches), len(client.committed), store.contextError)
	}
}

func TestRunDoesNotCommitWhenStoreFails(t *testing.T) {
	failure := errors.New("ClickHouse unavailable")
	client := &clientStub{polls: []func(context.Context) kgo.Fetches{
		func(context.Context) kgo.Fetches { return fetched(testRecord(t, 1)) },
	}}
	store := &storeStub{err: failure}
	err := testConsumer(t, client, store, 1, time.Second).Run(context.Background())
	if !errors.Is(err, failure) || len(client.committed) != 0 || client.allowedRebalance == 0 {
		t.Fatalf("Run error=%v, commits=%d, allowed=%d", err, len(client.committed), client.allowedRebalance)
	}
}

func TestRunReturnsInvalidRecordsWithoutCommit(t *testing.T) {
	for name, value := range map[string][]byte{
		"invalid JSON":  []byte("{"),
		"invalid event": []byte(`{"version":1,"event_id":"bad"}`),
	} {
		t.Run(name, func(t *testing.T) {
			record := &kgo.Record{Topic: "article-events", Partition: 2, Offset: 8, Value: value}
			client := &clientStub{polls: []func(context.Context) kgo.Fetches{
				func(context.Context) kgo.Fetches { return fetched(record) },
			}}
			store := &storeStub{}
			err := testConsumer(t, client, store, 1, time.Second).Run(context.Background())
			var invalid *InvalidRecordError
			if !errors.As(err, &invalid) || invalid.Record != record || len(store.batches) != 0 || len(client.committed) != 0 {
				t.Fatalf("Run error=%v, stored=%d, commits=%d", err, len(store.batches), len(client.committed))
			}
		})
	}
}

func TestRunDoesNotStoreOrCommitBatchWithInvalidRecord(t *testing.T) {
	valid := testRecord(t, 7)
	invalid := &kgo.Record{Topic: "article-events", Partition: 0, Offset: 8, Value: []byte("{")}
	client := &clientStub{polls: []func(context.Context) kgo.Fetches{
		func(context.Context) kgo.Fetches { return fetched(valid, invalid) },
	}}
	store := &storeStub{}
	err := testConsumer(t, client, store, 2, time.Second).Run(context.Background())
	var invalidRecord *InvalidRecordError
	if !errors.As(err, &invalidRecord) || invalidRecord.Record != invalid || len(store.batches) != 0 || len(client.committed) != 0 {
		t.Fatalf("Run error=%v, stored=%d, commits=%d", err, len(store.batches), len(client.committed))
	}
}

func TestRunReturnsCommitAndPollErrors(t *testing.T) {
	failure := errors.New("Kafka unavailable")
	for _, test := range []struct {
		name    string
		polls   []func(context.Context) kgo.Fetches
		commit  error
		message string
	}{
		{"commit", []func(context.Context) kgo.Fetches{func(context.Context) kgo.Fetches { return fetched(testRecord(t, 1)) }}, failure, "commit Kafka records"},
		{"poll", []func(context.Context) kgo.Fetches{func(context.Context) kgo.Fetches { return kgo.NewErrFetch(failure) }}, nil, "poll Kafka"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &clientStub{polls: test.polls, commitErr: test.commit}
			err := testConsumer(t, client, &storeStub{}, 1, time.Second).Run(context.Background())
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Run error = %v", err)
			}
		})
	}
}

func TestCloseReleasesRebalanceAndClient(t *testing.T) {
	client := &clientStub{}
	consumer := &Consumer{client: client}
	consumer.Close()
	if !client.closed || client.allowedRebalance != 1 {
		t.Fatalf("client closed=%t, allowed=%d", client.closed, client.allowedRebalance)
	}
}
