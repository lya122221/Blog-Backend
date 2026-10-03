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
	t.Setenv("ANALYTICS_PORT", "0")
	if err := run(); err == nil || !strings.Contains(err.Error(), "configure analytics service") {
		t.Fatalf("run error = %v", err)
	}

	t.Setenv("ANALYTICS_PORT", "")
	t.Setenv("LOG_FORMAT", "xml")
	if err := run(); err == nil || !strings.Contains(err.Error(), "configure logger") {
		t.Fatalf("run error = %v", err)
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
