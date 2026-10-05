package producer

import (
	"analytics/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Producer struct {
	client *kgo.Client
	topic  string
}

func NewProducer(brokers []string, topic string) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RecordDeliveryTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka client: %w", err)
	}
	return &Producer{client: client, topic: topic}, nil
}

func (producer *Producer) Publish(ctx context.Context, batch []models.Event) error {
	records, err := encodeRecords(producer.topic, batch)
	if err != nil {
		return err
	}
	if err := producer.client.ProduceSync(ctx, records...).FirstErr(); err != nil {
		return fmt.Errorf("publish events to Kafka: %w", err)
	}
	return nil
}

func encodeRecords(topic string, batch []models.Event) ([]*kgo.Record, error) {
	records := make([]*kgo.Record, 0, len(batch))
	for _, event := range batch {
		value, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("encode event: %w", err)
		}
		records = append(records, &kgo.Record{
			Topic: topic,
			Key:   []byte(event.ArticleID),
			Value: value,
		})
	}
	return records, nil
}

func (producer *Producer) Close() {
	producer.client.Close()
}
