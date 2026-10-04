package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("ANALYTICS_KAFKA_BROKERS", "localhost:9092")
	t.Setenv("ANALYTICS_KAFKA_TOPIC", "article-events")
	t.Setenv("ANALYTICS_KAFKA_CONSUMER_GROUP", "analytics-events")
	t.Setenv("ANALYTICS_KAFKA_BATCH_SIZE", "500")
	t.Setenv("ANALYTICS_KAFKA_FLUSH_INTERVAL", "1s")
	t.Setenv("ANALYTICS_CLICKHOUSE_ADDR", "localhost:9000")
	t.Setenv("ANALYTICS_CLICKHOUSE_DATABASE", "default")
	t.Setenv("ANALYTICS_CLICKHOUSE_USER", "default")
	t.Setenv("ANALYTICS_PORT", "0")
	if err := run(); err == nil || !strings.Contains(err.Error(), "configure analytics service") {
		t.Fatalf("run error = %v", err)
	}

	t.Setenv("ANALYTICS_PORT", "8082")
	t.Setenv("LOG_FORMAT", "xml")
	if err := run(); err == nil || !strings.Contains(err.Error(), "configure logger") {
		t.Fatalf("run error = %v", err)
	}
}

type workerStub struct {
	run func(context.Context) error
}

func (worker *workerStub) Run(ctx context.Context) error {
	return worker.run(ctx)
}

func TestServeAnalyticsStopsOnConsumerFailure(t *testing.T) {
	failure := errors.New("consumer failed")
	listener := newListenerStub(nil)
	worker := &workerStub{run: func(context.Context) error {
		<-listener.accepted
		return failure
	}}
	server := &http.Server{Handler: newRouter(testLogger(), func(*gin.Context) {}), ReadHeaderTimeout: time.Second}
	if err := serveAnalytics(context.Background(), server, listener, worker, testLogger()); !errors.Is(err, failure) {
		t.Fatalf("serveAnalytics error = %v", err)
	}
}

func TestServeAnalyticsStopsConsumerOnHTTPFailure(t *testing.T) {
	failure := errors.New("listener failed")
	listener := newListenerStub(failure)
	worker := &workerStub{run: func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}}
	server := &http.Server{Handler: newRouter(testLogger(), func(*gin.Context) {}), ReadHeaderTimeout: time.Second}
	if err := serveAnalytics(context.Background(), server, listener, worker, testLogger()); !errors.Is(err, failure) {
		t.Fatalf("serveAnalytics error = %v", err)
	}
}

func TestServeAnalyticsStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener := newListenerStub(nil)
	worker := &workerStub{run: func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}}
	server := &http.Server{Handler: newRouter(testLogger(), func(*gin.Context) {}), ReadHeaderTimeout: time.Second}
	go func() {
		<-listener.accepted
		cancel()
	}()
	if err := serveAnalytics(ctx, server, listener, worker, testLogger()); err != nil {
		t.Fatalf("serveAnalytics: %v", err)
	}
}

func TestServeHTTPShutsDownGracefully(t *testing.T) {
	listener := newListenerStub(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &http.Server{Handler: newRouter(testLogger(), func(*gin.Context) {}), ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() {
		done <- serveHTTP(ctx, server, listener, testLogger())
	}()

	select {
	case <-listener.accepted:
	case <-time.After(time.Second):
		t.Fatal("server did not start accepting connections")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveHTTP: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestServeHTTPReportsListenerFailure(t *testing.T) {
	serveErr := errors.New("listener failed")
	listener := newListenerStub(serveErr)
	server := &http.Server{Handler: newRouter(testLogger(), func(*gin.Context) {}), ReadHeaderTimeout: time.Second}
	err := serveHTTP(context.Background(), server, listener, testLogger())
	if err == nil || !strings.Contains(err.Error(), "serve HTTP") || !errors.Is(err, serveErr) {
		t.Fatalf("serveHTTP error = %v", err)
	}
}

type listenerStub struct {
	accepted  chan struct{}
	closed    chan struct{}
	acceptErr error
	startOnce sync.Once
	closeOnce sync.Once
}

func newListenerStub(acceptErr error) *listenerStub {
	return &listenerStub{
		accepted:  make(chan struct{}),
		closed:    make(chan struct{}),
		acceptErr: acceptErr,
	}
}

func (listener *listenerStub) Accept() (net.Conn, error) {
	listener.startOnce.Do(func() { close(listener.accepted) })
	if listener.acceptErr != nil {
		return nil, listener.acceptErr
	}
	<-listener.closed
	return nil, net.ErrClosed
}

func (listener *listenerStub) Close() error {
	listener.closeOnce.Do(func() { close(listener.closed) })
	return nil
}

func (listener *listenerStub) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8082}
}
