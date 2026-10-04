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

type DeadLetterPublisher interface {
	Publish(context.Context, *kgo.Record, error) error
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
	deadLetter    DeadLetterPublisher
	batchSize     int
	flushInterval time.Duration
	retryAttempts int
	retryBackoff  time.Duration
}

func NewConsumer(settings config.Config, preparer EventPreparer, store BatchStore, deadLetter DeadLetterPublisher) (*Consumer, error) {
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
		deadLetter:    deadLetter,
		batchSize:     settings.BatchSize,
		flushInterval: settings.FlushInterval,
		retryAttempts: settings.RetryAttempts,
		retryBackoff:  settings.RetryBackoff,
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
			eventErr := json.Unmarshal(record.Value, &event)
			var prepared models.ArticleEvent
			if eventErr == nil {
				prepared, eventErr = consumer.preparer.PrepareEvent(event)
			}
			if eventErr != nil {
				if len(records) > 0 {
					if err := consumer.flushPending(ctx, records, batch); err != nil {
						return err
					}
					records = nil
					batch = nil
				}
				invalid := &InvalidRecordError{Record: record, Err: eventErr}
				if err := consumer.retry(ctx, "publish invalid Kafka record to DLQ", func() error {
					return consumer.deadLetter.Publish(ctx, record, invalid)
				}); err != nil {
					return err
				}
				if err := consumer.commit(ctx, record); err != nil {
					return err
				}
				continue
			}
			if len(records) == 0 {
				flushAt = time.Now().Add(consumer.flushInterval)
			}
			records = append(records, record)
			batch = append(batch, prepared)
		}
		if len(records) > 0 && (timedOut || len(records) >= consumer.batchSize || !time.Now().Before(flushAt)) {
			if err := consumer.flushPending(ctx, records, batch); err != nil {
				return err
			}
			records = nil
			batch = nil
		}
		if len(records) == 0 {
			consumer.client.AllowRebalance()
		}
	}
}

func (consumer *Consumer) flushPending(ctx context.Context, records []*kgo.Record, batch []models.ArticleEvent) error {
	err := consumer.flush(ctx, records, batch)
	if err != nil && ctx.Err() != nil {
		return consumer.flushOnShutdown(ctx, records, batch)
	}
	return err
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
	if err := consumer.retry(ctx, "store Kafka events in ClickHouse", func() error {
		return consumer.store.StoreBatch(ctx, batch)
	}); err != nil {
		return err
	}
	return consumer.commit(ctx, records...)
}

func (consumer *Consumer) commit(ctx context.Context, records ...*kgo.Record) error {
	return consumer.retry(ctx, "commit Kafka records", func() error {
		return consumer.client.CommitRecords(ctx, records...)
	})
}

func (consumer *Consumer) retry(ctx context.Context, operation string, run func() error) error {
	var err error
	for attempt := 1; attempt <= consumer.retryAttempts; attempt++ {
		if ctx.Err() != nil {
			return fmt.Errorf("%s: %w", operation, ctx.Err())
		}
		if err = run(); err == nil {
			return nil
		}
		if attempt == consumer.retryAttempts {
			break
		}
		timer := time.NewTimer(consumer.retryBackoff * time.Duration(1<<(attempt-1)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%s: %w", operation, ctx.Err())
		case <-timer.C:
		}
	}
	return fmt.Errorf("%s after %d attempts: %w", operation, consumer.retryAttempts, err)
}

func (consumer *Consumer) Close() {
	consumer.client.AllowRebalance()
	consumer.client.Close()
}
