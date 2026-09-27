package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRunConfigurationErrors(t *testing.T) {
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_LEVEL", "invalid")
	if err := run(); err == nil || !strings.Contains(err.Error(), "configure logger") {
		t.Fatalf("run with invalid log level: %v", err)
	}

	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("AUTH_POSTGRES_HOST", "%")
	if err := run(); err == nil || !strings.Contains(err.Error(), "connect to storage") {
		t.Fatalf("run with invalid database host: %v", err)
	}

	t.Setenv("AUTH_POSTGRES_HOST", "")
	t.Setenv("AUTH_POSTGRES_DB", "%")
	if err := run(); err == nil || !strings.Contains(err.Error(), "connect to storage") {
		t.Fatalf("run with invalid database name: %v", err)
	}
}

func TestServeHTTPShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
	})

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- serveHTTP(ctx, server, listener, logger)
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d", response.StatusCode)
	}

	cancel()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("serveHTTP: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestServeHTTPListenerError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := serveHTTP(context.Background(), server, listener, logger); err == nil {
		t.Fatal("serveHTTP with closed listener succeeded")
	}
}

func TestServeHTTPExternallyClosed(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		ReadHeaderTimeout: 5 * time.Second,
	}
	t.Cleanup(func() { _ = server.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- serveHTTP(context.Background(), server, listener, logger)
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("serveHTTP after close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}
