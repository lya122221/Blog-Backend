package workers

import (
	"blog/internal/config"
	"blog/internal/models"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type outboxTestDriver struct{ state *outboxTxState }

func (d outboxTestDriver) Open(string) (driver.Conn, error) {
	return &outboxTestConn{state: d.state}, nil
}

type outboxTestConnector struct{ state *outboxTxState }

func (c outboxTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &outboxTestConn{state: c.state}, nil
}
func (c outboxTestConnector) Driver() driver.Driver { return outboxTestDriver(c) }

type outboxTxState struct {
	commits, rollbacks int
	commitDone         chan struct{}
}
type outboxTestConn struct{ state *outboxTxState }

func (c *outboxTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *outboxTestConn) Close() error { return nil }
func (c *outboxTestConn) Begin() (driver.Tx, error) {
	return &outboxTestTx{state: c.state}, nil
}

type outboxTestTx struct{ state *outboxTxState }

func (tx *outboxTestTx) Commit() error {
	tx.state.commits++
	if tx.state.commitDone != nil {
		select {
		case tx.state.commitDone <- struct{}{}:
		default:
		}
	}
	return nil
}
func (tx *outboxTestTx) Rollback() error {
	tx.state.rollbacks++
	return nil
}

type outboxRepoStub struct {
	db          *sql.DB
	events      []models.OutboxEvent
	marked      []string
	markErr     error
	cleanupDone chan struct{}
}

func (r *outboxRepoStub) BeginOutboxTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}
func (r *outboxRepoStub) ReadPendingOutboxEvents(context.Context, *sql.Tx, int) ([]models.OutboxEvent, error) {
	if len(r.marked) > 0 {
		return nil, nil
	}
	return r.events, nil
}
func (r *outboxRepoStub) MarkOutboxEventPublished(_ context.Context, _ *sql.Tx, id string, publishedAt time.Time) error {
	if publishedAt.IsZero() {
		return errors.New("missing publication time")
	}
	if r.markErr != nil {
		return r.markErr
	}
	r.marked = append(r.marked, id)
	return nil
}
func (r *outboxRepoStub) DeletePublishedOutboxEventsBefore(context.Context, time.Time, int) (int64, error) {
	if r.cleanupDone != nil {
		select {
		case r.cleanupDone <- struct{}{}:
		default:
		}
	}
	return 0, nil
}

type outboxPublisherStub struct {
	published []string
	err       error
	failures  int
	attempts  int
}

func (p *outboxPublisherStub) Publish(_ context.Context, event models.OutboxEvent) error {
	p.attempts++
	if p.failures > 0 {
		p.failures--
		return errors.New("temporary Kafka failure")
	}
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, event.ID)
	return nil
}

func newOutboxTestWorker(t *testing.T, events []models.OutboxEvent) (*OutboxWorker, *outboxRepoStub, *outboxPublisherStub, *outboxTxState) {
	t.Helper()
	state := &outboxTxState{}
	db := sql.OpenDB(outboxTestConnector{state: state})
	t.Cleanup(func() { _ = db.Close() })
	repo := &outboxRepoStub{db: db, events: events}
	publisher := &outboxPublisherStub{}
	worker := NewOutboxWorker(repo, publisher, slog.New(slog.NewTextHandler(io.Discard, nil)), config.Outbox{
		BatchSize: 10, PollInterval: time.Millisecond, RetryInterval: time.Millisecond,
		CleanupInterval: time.Hour, Retention: 7 * 24 * time.Hour,
	})
	return worker, repo, publisher, state
}

func TestOutboxWorkerPublishesBeforeCommit(t *testing.T) {
	worker, repo, publisher, state := newOutboxTestWorker(t, []models.OutboxEvent{{ID: "first"}, {ID: "second"}})
	count, err := worker.publishBatch(context.Background())
	if err != nil || count != 2 || state.commits != 1 || state.rollbacks != 0 {
		t.Fatalf("publish batch: count=%d, commits=%d, rollbacks=%d, err=%v", count, state.commits, state.rollbacks, err)
	}
	if len(publisher.published) != 2 || len(repo.marked) != 2 || publisher.published[0] != repo.marked[0] || publisher.published[1] != repo.marked[1] {
		t.Fatalf("published=%v, marked=%v", publisher.published, repo.marked)
	}
}

func TestOutboxWorkerRollsBackAfterFailure(t *testing.T) {
	for _, failure := range []string{"publish", "mark"} {
		t.Run(failure, func(t *testing.T) {
			worker, repo, publisher, state := newOutboxTestWorker(t, []models.OutboxEvent{{ID: "first"}})
			if failure == "publish" {
				publisher.err = errors.New("Kafka unavailable")
			} else {
				repo.markErr = errors.New("database unavailable")
			}
			if _, err := worker.publishBatch(context.Background()); err == nil {
				t.Fatal("expected publish batch error")
			}
			if state.commits != 0 || state.rollbacks != 1 || len(repo.marked) != 0 {
				t.Fatalf("commits=%d, rollbacks=%d, marked=%v", state.commits, state.rollbacks, repo.marked)
			}
		})
	}
}

func TestOutboxWorkerStopsAndCleansPublishedEvents(t *testing.T) {
	worker, repo, _, _ := newOutboxTestWorker(t, nil)
	repo.cleanupDone = make(chan struct{}, 1)
	worker.config.CleanupInterval = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	select {
	case <-repo.cleanupDone:
	case <-time.After(time.Second):
		t.Fatal("published outbox cleanup did not run")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("outbox worker did not stop")
	}
}

func TestOutboxWorkerRetriesTemporaryPublishFailure(t *testing.T) {
	worker, repo, publisher, state := newOutboxTestWorker(t, []models.OutboxEvent{{ID: "first"}})
	state.commitDone = make(chan struct{}, 1)
	publisher.failures = 1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	select {
	case <-state.commitDone:
	case <-time.After(time.Second):
		t.Fatal("outbox event was not retried")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if publisher.attempts != 2 || state.rollbacks < 1 || state.commits != 1 || len(repo.marked) != 1 {
		t.Fatalf("attempts=%d, commits=%d, rollbacks=%d, marked=%v", publisher.attempts, state.commits, state.rollbacks, repo.marked)
	}
}
