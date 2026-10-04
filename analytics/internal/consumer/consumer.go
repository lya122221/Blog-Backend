package consumer

import (
	"analytics/internal/config"
	"analytics/internal/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const shutdownTimeout = 10 * time.Second

type EventPreparer interface {
	PrepareEvent(models.Event) (models.ArticleEvent, error)
}

type BatchStore interface {
	StoreBatch(context.Context, []models.ArticleEvent) error
}

type kafkaClient interface {
	PollRecords(context.Context, int) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	AllowRebalance()
	Close()
}

type InvalidRecordError struct {
	Record *kgo.Record
	Err    error
}

func (err *InvalidRecordError) Error() string {
	return fmt.Sprintf("invalid Kafka record %s[%d]@%d: %v", err.Record.Topic, err.Record.Partition, err.Record.Offset, err.Err)
}

func (err *InvalidRecordError) Unwrap() error {
	return err.Err
}

type Consumer struct {
	client        kafkaClient
	preparer      EventPreparer
	store         BatchStore
	batchSize     int
	flushInterval time.Duration
}

func NewConsumer(settings config.Config, preparer EventPreparer, store BatchStore) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(settings.KafkaBrokers...),
		kgo.ConsumeTopics(settings.KafkaTopic),
		kgo.ConsumerGroup(settings.KafkaGroup),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka consumer: %w", err)
	}
	return &Consumer{
		client:        client,
		preparer:      preparer,
		store:         store,
		batchSize:     settings.BatchSize,
		flushInterval: settings.FlushInterval,
	}, nil
}

func (consumer *Consumer) Run(ctx context.Context) error {
	var records []*kgo.Record
	var batch []models.ArticleEvent
	var flushAt time.Time
	defer consumer.client.AllowRebalance()

	for {
		if ctx.Err() != nil {
			return consumer.flushOnShutdown(ctx, records, batch)
		}
		pollCtx := ctx
		cancelPoll := func() {}
		if len(records) > 0 {
			pollCtx, cancelPoll = context.WithDeadline(ctx, flushAt)
		}
		fetches := consumer.client.PollRecords(pollCtx, consumer.batchSize-len(records))
		cancelPoll()
		if ctx.Err() != nil {
			return consumer.flushOnShutdown(ctx, records, batch)
		}

		timedOut := false
		for _, fetchErr := range fetches.Errors() {
			if errors.Is(fetchErr.Err, context.DeadlineExceeded) && len(records) > 0 {
				timedOut = true
				continue
			}
			return fmt.Errorf("poll Kafka %s[%d]: %w", fetchErr.Topic, fetchErr.Partition, fetchErr.Err)
		}
		for _, record := range fetches.Records() {
			var event models.Event
			if err := json.Unmarshal(record.Value, &event); err != nil {
				return &InvalidRecordError{Record: record, Err: err}
			}
			prepared, err := consumer.preparer.PrepareEvent(event)
			if err != nil {
				return &InvalidRecordError{Record: record, Err: err}
			}
			if len(records) == 0 {
				flushAt = time.Now().Add(consumer.flushInterval)
			}
			records = append(records, record)
			batch = append(batch, prepared)
		}
		if len(records) > 0 && (timedOut || len(records) >= consumer.batchSize || !time.Now().Before(flushAt)) {
			if err := consumer.flush(ctx, records, batch); err != nil {
				return err
			}
			records = nil
			batch = nil
		}
	}
}

func (consumer *Consumer) flushOnShutdown(ctx context.Context, records []*kgo.Record, batch []models.ArticleEvent) error {
	if len(records) == 0 {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	return consumer.flush(shutdownCtx, records, batch)
}

func (consumer *Consumer) flush(ctx context.Context, records []*kgo.Record, batch []models.ArticleEvent) error {
	if err := consumer.store.StoreBatch(ctx, batch); err != nil {
		return fmt.Errorf("store Kafka events in ClickHouse: %w", err)
	}
	if err := consumer.client.CommitRecords(ctx, records...); err != nil {
		return fmt.Errorf("commit Kafka records: %w", err)
	}
	consumer.client.AllowRebalance()
	return nil
}

func (consumer *Consumer) Close() {
	consumer.client.AllowRebalance()
	consumer.client.Close()
}
