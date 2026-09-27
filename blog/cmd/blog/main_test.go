package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type serverStub struct {
	shutdownErr    error
	closeErr       error
	shutdownCalled bool
	closeCalled    bool
}

func (s *serverStub) Shutdown(context.Context) error {
	s.shutdownCalled = true
	return s.shutdownErr
}

func (s *serverStub) Close() error {
	s.closeCalled = true
	return s.closeErr
}

func serverResult(err error) <-chan error {
	done := make(chan error, 1)
	done <- err
	return done
}

func TestShutdownHTTPServer(t *testing.T) {
	server := &serverStub{}
	err := shutdownHTTPServer(server, serverResult(http.ErrServerClosed), time.Second)
	if err != nil {
		t.Fatalf("shutdownHTTPServer: %v", err)
	}
	if !server.shutdownCalled || server.closeCalled {
		t.Fatalf("unexpected calls: shutdown=%v close=%v", server.shutdownCalled, server.closeCalled)
	}
}

func TestShutdownHTTPServerForcesClose(t *testing.T) {
	server := &serverStub{shutdownErr: context.DeadlineExceeded}
	err := shutdownHTTPServer(server, serverResult(http.ErrServerClosed), time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v", err)
	}
	if !server.closeCalled {
		t.Fatal("server was not forcibly closed after shutdown timeout")
	}
}

func TestShutdownHTTPServerCombinesErrors(t *testing.T) {
	shutdownErr := errors.New("shutdown failed")
	closeErr := errors.New("close failed")
	serveErr := errors.New("serve failed")
	server := &serverStub{shutdownErr: shutdownErr, closeErr: closeErr}

	err := shutdownHTTPServer(server, serverResult(serveErr), time.Second)
	for _, expected := range []error{shutdownErr, closeErr, serveErr} {
		if !errors.Is(err, expected) {
			t.Fatalf("combined error %v does not contain %v", err, expected)
		}
	}
}

func TestWaitForWorker(t *testing.T) {
	done := make(chan error, 1)
	done <- nil
	if err := waitForWorker(done, time.Second); err != nil {
		t.Fatalf("waitForWorker: %v", err)
	}

	workerErr := errors.New("worker failed")
	done = make(chan error, 1)
	done <- workerErr
	if err := waitForWorker(done, time.Second); !errors.Is(err, workerErr) {
		t.Fatalf("worker error = %v", err)
	}

	if err := waitForWorker(make(chan error), time.Millisecond); err == nil {
		t.Fatal("expected worker timeout")
	}
}
