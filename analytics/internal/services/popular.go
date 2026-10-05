package services

import (
	"analytics/internal/models"
	"context"
	"fmt"
	"time"
)

type PopularRepository interface {
	ListPopular(context.Context, time.Time, time.Time, int) ([]models.PopularArticle, error)
}

type PopularService struct {
	repository PopularRepository
	now        func() time.Time
}

func NewPopularService(repository PopularRepository) *PopularService {
	return &PopularService{repository: repository, now: time.Now}
}

func (service *PopularService) Popular(ctx context.Context, window string, limit int) ([]models.PopularArticle, error) {
	var duration time.Duration
	switch window {
	case "5m":
		duration = 5 * time.Minute
	case "1h":
		duration = time.Hour
	case "1d":
		duration = 24 * time.Hour
	default:
		return nil, &ValidationError{message: "window must be 5m, 1h or 1d"}
	}
	if limit < 1 || limit > 100 {
		return nil, &ValidationError{message: "limit must be between 1 and 100"}
	}
	until := service.now().UTC().Truncate(time.Microsecond)
	articles, err := service.repository.ListPopular(ctx, until.Add(-duration), until, limit)
	if err != nil {
		return nil, fmt.Errorf("list popular articles: %w", err)
	}
	return articles, nil
}
