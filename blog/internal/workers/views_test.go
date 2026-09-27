package workers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"uuid"
)

type viewsRepositoryStub struct {
	views     map[string]int
	updated   map[uuid.UUID]int
	readErr   error
	readCalls int
}

func (r *viewsRepositoryStub) GetAndClearViewsCount() (map[string]int, error) {
	r.readCalls++
	return r.views, r.readErr
}

func (r *viewsRepositoryStub) UpdateArticleViews(viewsCount int, articleID uuid.UUID) error {
	r.updated[articleID] = viewsCount
	return nil
}

func TestUpdateViews(t *testing.T) {
	articleID := uuid.NewV4()
	repo := &viewsRepositoryStub{
		views:   map[string]int{fmt.Sprint(articleID): 3, "invalid-uuid": 2},
		updated: make(map[uuid.UUID]int),
	}

	if err := updateViews(repo); err != nil {
		t.Fatalf("update views: %v", err)
	}
	if repo.readCalls != 1 || len(repo.updated) != 1 || repo.updated[articleID] != 3 {
		t.Fatalf("unexpected view updates: %+v", repo)
	}
}

func TestUpdateViewsReadError(t *testing.T) {
	readErr := errors.New("redis unavailable")
	repo := &viewsRepositoryStub{readErr: readErr}

	if err := updateViews(repo); !errors.Is(err, readErr) {
		t.Fatalf("update views error = %v, want %v", err, readErr)
	}
}

func TestViewsUpdaterWorkerStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &viewsRepositoryStub{}

	if err := StartViewsUpdaterWorker(ctx, repo); err != nil {
		t.Fatalf("stop worker: %v", err)
	}
	if repo.readCalls != 0 {
		t.Fatalf("worker read views after cancellation: %d", repo.readCalls)
	}
}
