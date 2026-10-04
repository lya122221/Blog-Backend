package producer

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type DeadLetter struct {
	client *kgo.Client
	topic  string
}

func NewDeadLetter(brokers []string, topic string) (*DeadLetter, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RecordDeliveryTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka DLQ producer: %w", err)
	}
	return &DeadLetter{client: client, topic: topic}, nil
}

func (producer *DeadLetter) Publish(ctx context.Context, source *kgo.Record, reason error) error {
	record := deadLetterRecord(producer.topic, source, reason)
	if err := producer.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("publish Kafka DLQ record: %w", err)
	}
	return nil
}

func (producer *DeadLetter) Close() {
	producer.client.Close()
}

func deadLetterRecord(topic string, source *kgo.Record, reason error) *kgo.Record {
	return &kgo.Record{
		Topic: topic,
		Key:   append([]byte(nil), source.Key...),
		Value: append([]byte(nil), source.Value...),
		Headers: []kgo.RecordHeader{
			{Key: "source_topic", Value: []byte(source.Topic)},
			{Key: "source_partition", Value: []byte(strconv.FormatInt(int64(source.Partition), 10))},
			{Key: "source_offset", Value: []byte(strconv.FormatInt(source.Offset, 10))},
			{Key: "error", Value: []byte(reason.Error())},
		},
	}
}
