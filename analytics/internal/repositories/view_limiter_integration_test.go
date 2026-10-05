package repositories

import (
	"analytics/internal/models"
	"context"
	"os"
	"testing"
	"time"
	"uuid"
)

func TestViewLimiterIntegration(t *testing.T) {
	addr := os.Getenv("ANALYTICS_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set ANALYTICS_TEST_REDIS_ADDR to run Redis integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	limiter, err := NewViewLimiter(ctx, addr)
	if err != nil {
		t.Fatalf("connect to Redis: %v", err)
	}
	t.Cleanup(func() {
		if err := limiter.Close(); err != nil {
			t.Errorf("close Redis connection: %v", err)
		}
	})
	event := models.Event{
		ID: uuid.New().String(), Type: models.ArticleOpened,
		ArticleID: uuid.New().String(), VisitorID: uuid.New().String(),
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := limiter.client.Del(cleanupCtx, viewKey(event)).Err(); err != nil {
			t.Errorf("delete test key: %v", err)
		}
	})
	if accepted, err := limiter.Acquire(ctx, event); err != nil || !accepted {
		t.Fatalf("first view: accepted = %v, error = %v", accepted, err)
	}
	ttl, err := limiter.client.TTL(ctx, viewKey(event)).Result()
	if err != nil || ttl <= 0 || ttl > viewWindow {
		t.Fatalf("key TTL = %v, error = %v", ttl, err)
	}
	other := event
	other.ID = uuid.New().String()
	if accepted, err := limiter.Acquire(ctx, other); err != nil || accepted {
		t.Fatalf("repeated view: accepted = %v, error = %v", accepted, err)
	}
	if err := limiter.Release(ctx, other); err != nil {
		t.Fatalf("release unrelated event: %v", err)
	}
	if accepted, err := limiter.Acquire(ctx, other); err != nil || accepted {
		t.Fatalf("reservation removed by unrelated event: accepted = %v, error = %v", accepted, err)
	}
	if err := limiter.Release(ctx, event); err != nil {
		t.Fatalf("release original event: %v", err)
	}
	if accepted, err := limiter.Acquire(ctx, other); err != nil || !accepted {
		t.Fatalf("view after release: accepted = %v, error = %v", accepted, err)
	}
}
