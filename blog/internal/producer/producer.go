package producer

import (
	"blog/internal/models"
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Producer struct {
	client kafkaClient
	topic  string
}

type kafkaClient interface {
	ProduceSync(context.Context, ...*kgo.Record) kgo.ProduceResults
	Close()
}

func NewProducer(brokers []string, topic string) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RecordDeliveryTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka producer: %w", err)
	}
	return &Producer{client: client, topic: topic}, nil
}

func (producer *Producer) Publish(ctx context.Context, event models.OutboxEvent) error {
	if err := producer.client.ProduceSync(ctx, recordForEvent(producer.topic, event)).FirstErr(); err != nil {
		return fmt.Errorf("publish outbox event %s: %w", event.ID, err)
	}
	return nil
}

func recordForEvent(topic string, event models.OutboxEvent) *kgo.Record {
	return &kgo.Record{
		Topic: topic,
		Key:   []byte(event.ArticleID),
		Value: append([]byte(nil), event.Payload...),
	}
}

func (producer *Producer) Close() {
	producer.client.Close()
}
