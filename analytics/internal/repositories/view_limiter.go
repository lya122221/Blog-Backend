package repositories

import (
	"analytics/internal/models"
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const viewWindow = 30 * time.Minute

var releaseViewScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)

type ViewLimiter struct {
	client *redis.Client
}

func NewViewLimiter(ctx context.Context, addr string) (*ViewLimiter, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping analytics Redis: %w", err)
	}
	return &ViewLimiter{client: client}, nil
}

func (limiter *ViewLimiter) Acquire(ctx context.Context, event models.Event) (bool, error) {
	accepted, err := limiter.client.SetNX(ctx, viewKey(event), event.ID, viewWindow).Result()
	if err != nil {
		return false, fmt.Errorf("reserve view in Redis: %w", err)
	}
	return accepted, nil
}

func (limiter *ViewLimiter) Release(ctx context.Context, event models.Event) error {
	if err := releaseViewScript.Run(ctx, limiter.client, []string{viewKey(event)}, event.ID).Err(); err != nil {
		return fmt.Errorf("release view in Redis: %w", err)
	}
	return nil
}

func (limiter *ViewLimiter) Close() error {
	return limiter.client.Close()
}

func viewKey(event models.Event) string {
	return "analytics:view:" + event.VisitorID + ":" + event.ArticleID + ":" + string(event.Type)
}
