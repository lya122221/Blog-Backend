package producer

import (
	"blog/internal/models"
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

type producerClientStub struct {
	record *kgo.Record
	err    error
}

func (c *producerClientStub) ProduceSync(_ context.Context, records ...*kgo.Record) kgo.ProduceResults {
	c.record = records[0]
	return kgo.ProduceResults{{Record: records[0], Err: c.err}}
}
func (c *producerClientStub) Close() {}

func TestRecordForEvent(t *testing.T) {
	event := models.OutboxEvent{
		ID:        "event-id",
		ArticleID: "article-id",
		Payload:   []byte(`{"event_id":"event-id"}`),
	}
	record := recordForEvent("article-events", event)
	if record.Topic != "article-events" || string(record.Key) != event.ArticleID || !bytes.Equal(record.Value, event.Payload) {
		t.Fatalf("incorrect Kafka record: %+v", record)
	}
	event.Payload[0] = '['
	if record.Value[0] == '[' {
		t.Fatal("Kafka record shares the outbox payload buffer")
	}
}

func TestProducerWaitsForKafkaResult(t *testing.T) {
	client := &producerClientStub{}
	producer := &Producer{client: client, topic: "article-events"}
	event := models.OutboxEvent{ID: "event-id", ArticleID: "article-id", Payload: []byte(`{}`)}
	if err := producer.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if client.record == nil || string(client.record.Key) != event.ArticleID || string(client.record.Value) != string(event.Payload) {
		t.Fatalf("produced record = %+v", client.record)
	}
	client.err = errors.New("Kafka unavailable")
	if err := producer.Publish(context.Background(), event); !errors.Is(err, client.err) {
		t.Fatalf("expected Kafka error, got %v", err)
	}
}
