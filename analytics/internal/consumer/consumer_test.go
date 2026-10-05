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
	commitErrs       []error
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
	if len(client.commitErrs) > 0 {
		err := client.commitErrs[0]
		client.commitErrs = client.commitErrs[1:]
		return err
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
	errs         []error
	contextError error
	onStore      func()
}

func (store *storeStub) StoreBatch(ctx context.Context, batch []models.ArticleEvent) error {
	store.contextError = ctx.Err()
	store.batches = append(store.batches, append([]models.ArticleEvent(nil), batch...))
	if store.onStore != nil {
		store.onStore()
	}
	if len(store.errs) > 0 {
		err := store.errs[0]
		store.errs = store.errs[1:]
		return err
	}
	return store.err
}

type deadLetterStub struct {
	records []*kgo.Record
	reasons []error
	err     error
	errs    []error
	onSend  func()
}

func (stub *deadLetterStub) Publish(_ context.Context, record *kgo.Record, reason error) error {
	stub.records = append(stub.records, record)
	stub.reasons = append(stub.reasons, reason)
	if stub.onSend != nil {
		stub.onSend()
	}
	if len(stub.errs) > 0 {
		err := stub.errs[0]
		stub.errs = stub.errs[1:]
		return err
	}
	return stub.err
}

func testConsumer(t *testing.T, client *clientStub, store *storeStub, batchSize int, flushInterval time.Duration) *Consumer {
	t.Helper()
	preparer, err := services.NewEventsService(models.DefaultWeights())
	if err != nil {
		t.Fatalf("NewEventsService: %v", err)
	}
	return &Consumer{
		client: client, preparer: preparer, store: store, deadLetter: &deadLetterStub{},
		batchSize: batchSize, flushInterval: flushInterval, retryAttempts: 3, retryBackoff: time.Millisecond,
	}
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

func TestRunPublishesInvalidRecordsToDLQ(t *testing.T) {
	for name, value := range map[string][]byte{
		"invalid JSON":  []byte("{"),
		"invalid event": []byte(`{"version":1,"event_id":"bad"}`),
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			record := &kgo.Record{Topic: "article-events", Partition: 2, Offset: 8, Value: value}
			client := &clientStub{polls: []func(context.Context) kgo.Fetches{
				func(context.Context) kgo.Fetches { return fetched(record) },
			}, onCommit: cancel}
			store := &storeStub{}
			consumer := testConsumer(t, client, store, 1, time.Second)
			deadLetter := consumer.deadLetter.(*deadLetterStub)
			err := consumer.Run(ctx)
			var invalid *InvalidRecordError
			if err != nil || len(deadLetter.records) != 1 || deadLetter.records[0] != record || !errors.As(deadLetter.reasons[0], &invalid) || invalid.Record != record || len(store.batches) != 0 || len(client.committed) != 1 || client.committed[0][0] != record {
				t.Fatalf("Run error=%v, DLQ=%d, stored=%d, commits=%d", err, len(deadLetter.records), len(store.batches), len(client.committed))
			}
		})
	}
}

func TestRunStoresValidRecordsAroundInvalidRecord(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	valid := testRecord(t, 7)
	invalid := &kgo.Record{Topic: "article-events", Partition: 0, Offset: 8, Value: []byte("{")}
	after := testRecord(t, 9)
	var calls []string
	client := &clientStub{polls: []func(context.Context) kgo.Fetches{
		func(context.Context) kgo.Fetches { return fetched(valid, invalid, after) },
		func(context.Context) kgo.Fetches {
			cancel()
			return kgo.NewErrFetch(context.Canceled)
		},
	}, onCommit: func() { calls = append(calls, "commit") }}
	store := &storeStub{onStore: func() { calls = append(calls, "store") }}
	consumer := testConsumer(t, client, store, 3, time.Second)
	deadLetter := consumer.deadLetter.(*deadLetterStub)
	deadLetter.onSend = func() { calls = append(calls, "DLQ") }
	if err := consumer.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.batches) != 2 || len(store.batches[0]) != 1 || len(store.batches[1]) != 1 || len(deadLetter.records) != 1 || deadLetter.records[0] != invalid || len(client.committed) != 3 {
		t.Fatalf("stored=%d, DLQ=%d, commits=%d", len(store.batches), len(deadLetter.records), len(client.committed))
	}
	if client.committed[0][0] != valid || client.committed[1][0] != invalid || client.committed[2][0] != after {
		t.Fatalf("committed records = %+v", client.committed)
	}
	want := []string{"store", "commit", "DLQ", "commit", "store", "commit"}
	if len(calls) != len(want) {
		t.Fatalf("order = %v", calls)
	}
	for index := range want {
		if calls[index] != want[index] {
			t.Fatalf("order = %v, want %v", calls, want)
		}
	}
}

func TestRunDoesNotCommitInvalidRecordWhenDLQFails(t *testing.T) {
	failure := errors.New("DLQ unavailable")
	valid := testRecord(t, 7)
	invalid := &kgo.Record{Topic: "article-events", Partition: 0, Offset: 8, Value: []byte("{")}
	client := &clientStub{polls: []func(context.Context) kgo.Fetches{
		func(context.Context) kgo.Fetches { return fetched(valid, invalid) },
	}}
	store := &storeStub{}
	consumer := testConsumer(t, client, store, 2, time.Second)
	deadLetter := consumer.deadLetter.(*deadLetterStub)
	deadLetter.err = failure
	err := consumer.Run(context.Background())
	if !errors.Is(err, failure) || len(deadLetter.records) != 3 || len(store.batches) != 1 || len(client.committed) != 1 || client.committed[0][0] != valid {
		t.Fatalf("Run error=%v, DLQ=%d, stored=%d, commits=%d", err, len(deadLetter.records), len(store.batches), len(client.committed))
	}
}

func TestRunRetriesTemporaryStoreCommitAndDLQFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("temporary failure")
	valid := testRecord(t, 7)
	invalid := &kgo.Record{Topic: "article-events", Partition: 0, Offset: 8, Value: []byte("{")}
	client := &clientStub{polls: []func(context.Context) kgo.Fetches{
		func(context.Context) kgo.Fetches { return fetched(valid, invalid) },
	}, commitErrs: []error{failure, nil, failure, nil}}
	client.onCommit = func() {
		if len(client.committed) == 4 {
			cancel()
		}
	}
	store := &storeStub{errs: []error{failure, nil}}
	consumer := testConsumer(t, client, store, 2, time.Second)
	deadLetter := consumer.deadLetter.(*deadLetterStub)
	deadLetter.errs = []error{failure, nil}
	if err := consumer.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.batches) != 2 || len(deadLetter.records) != 2 || len(client.committed) != 4 || client.committed[3][0] != invalid {
		t.Fatalf("stored=%d, DLQ=%d, commits=%d", len(store.batches), len(deadLetter.records), len(client.committed))
	}
}

func TestRetryStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{retryAttempts: 3, retryBackoff: time.Second}
	attempts := 0
	err := consumer.retry(ctx, "store batch", func() error {
		attempts++
		cancel()
		return errors.New("temporary failure")
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("retry error=%v, attempts=%d", err, attempts)
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
