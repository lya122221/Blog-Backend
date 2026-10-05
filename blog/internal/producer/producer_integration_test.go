package producer

import (
	"blog/internal/models"
	"context"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestProducerKafka(t *testing.T) {
	brokers := os.Getenv("BLOG_TEST_KAFKA_BROKERS")
	topic := os.Getenv("BLOG_TEST_KAFKA_TOPIC")
	if brokers == "" || topic == "" {
		t.Skip("set BLOG_TEST_KAFKA_BROKERS and BLOG_TEST_KAFKA_TOPIC to run the Kafka integration test")
	}
	addresses := strings.Split(brokers, ",")
	producer, err := NewProducer(addresses, topic)
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	event := models.OutboxEvent{
		ID:        uuid.NewV4().String(),
		ArticleID: uuid.NewV4().String(),
		Payload:   []byte(`{"version":1,"type":"article.created"}`),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := producer.Publish(ctx, event); err != nil {
		t.Fatal(err)
	}
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(addresses...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	for ctx.Err() == nil {
		fetches := consumer.PollRecords(ctx, 100)
		if err := fetches.Err(); err != nil {
			t.Fatal(err)
		}
		for _, record := range fetches.Records() {
			if string(record.Key) == event.ArticleID {
				if string(record.Value) != string(event.Payload) {
					t.Fatalf("Kafka payload = %s", record.Value)
				}
				return
			}
		}
	}
	t.Fatalf("Kafka did not return event for article %s: %v", event.ArticleID, ctx.Err())
}
