package repositories

import (
	"analytics/internal/models"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type batchStub struct {
	driver.Batch
	rows      [][]any
	appendErr error
	sendErr   error
	sent      bool
	closed    bool
}

func (batch *batchStub) Append(values ...any) error {
	if batch.appendErr != nil {
		return batch.appendErr
	}
	batch.rows = append(batch.rows, values)
	return nil
}

func (batch *batchStub) Send() error {
	batch.sent = true
	return batch.sendErr
}

func (batch *batchStub) Close() error {
	batch.closed = true
	return nil
}

type connStub struct {
	driver.Conn
	batch      driver.Batch
	prepareErr error
	query      string
	prepared   bool
}

func (conn *connStub) PrepareBatch(_ context.Context, query string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
	conn.prepared = true
	conn.query = query
	return conn.batch, conn.prepareErr
}

func TestStoreBatchMapsEvents(t *testing.T) {
	batch := &batchStub{}
	conn := &connStub{batch: batch}
	repository := &ClickHouse{conn: conn}
	eventID := uuid.New().String()
	articleID := uuid.New().String()
	authorID := uuid.New().String()
	visitorID := uuid.New().String()
	occurredAt := time.Date(2026, time.October, 3, 12, 30, 0, 123456000, time.UTC)
	tags := []string{"go", "analytics"}

	err := repository.StoreBatch(context.Background(), []models.ArticleEvent{{
		Event: models.Event{
			ID:         eventID,
			Type:       models.ArticleOpened,
			ArticleID:  articleID,
			AuthorID:   authorID,
			VisitorID:  visitorID,
			OccurredAt: occurredAt,
			Title:      "Article title",
			Tags:       tags,
		},
		ScoreDelta: 3,
	}})
	if err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}
	if !conn.prepared || conn.query != insertArticleEvents || !batch.sent || !batch.closed {
		t.Fatalf("unexpected batch state: prepared=%t query=%q sent=%t closed=%t", conn.prepared, conn.query, batch.sent, batch.closed)
	}
	want := []any{eventID, string(models.ArticleOpened), articleID, authorID, nil, visitorID, occurredAt, int16(3), "Article title", tags}
	if len(batch.rows) != 1 || !reflect.DeepEqual(batch.rows[0], want) {
		t.Fatalf("batch rows = %#v, want %#v", batch.rows, want)
	}
}

func TestStoreBatchSkipsEmptyBatch(t *testing.T) {
	conn := &connStub{}
	if err := (&ClickHouse{conn: conn}).StoreBatch(context.Background(), nil); err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}
	if conn.prepared {
		t.Fatal("empty batch prepared an insert")
	}
}

func TestStoreBatchReturnsErrors(t *testing.T) {
	failure := errors.New("ClickHouse unavailable")
	events := []models.ArticleEvent{{Event: models.Event{ID: uuid.New().String()}}}
	for _, test := range []struct {
		name      string
		conn      *connStub
		batch     *batchStub
		wantError string
	}{
		{name: "prepare", conn: &connStub{prepareErr: failure}, wantError: "prepare ClickHouse batch"},
		{name: "append", batch: &batchStub{appendErr: failure}, wantError: "append event"},
		{name: "send", batch: &batchStub{sendErr: failure}, wantError: "send ClickHouse batch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.conn == nil {
				test.conn = &connStub{batch: test.batch}
			}
			err := (&ClickHouse{conn: test.conn}).StoreBatch(context.Background(), events)
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("StoreBatch error = %v", err)
			}
			if test.batch != nil && !test.batch.closed {
				t.Fatal("failed batch was not closed")
			}
		})
	}
}
