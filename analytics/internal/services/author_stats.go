package services

import (
	"analytics/internal/models"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrArticleNotFound = errors.New("article not found")

type AuthorStatsRepository interface {
	ArticleMetadata(context.Context, string) (*models.ArticleMetadata, error)
	AuthorStats(context.Context, string, string, time.Time, time.Time, bool) ([]models.StatsBucket, error)
}

type AuthorStatsService struct {
	repository AuthorStatsRepository
	now        func() time.Time
}

func NewAuthorStatsService(repository AuthorStatsRepository) *AuthorStatsService {
	return &AuthorStatsService{repository: repository, now: time.Now}
}

func (service *AuthorStatsService) AuthorStats(ctx context.Context, authorID, period string) (models.AuthorStats, error) {
	return service.stats(ctx, authorID, "", period, service.now().UTC().Truncate(time.Microsecond))
}

func (service *AuthorStatsService) ArticleStats(ctx context.Context, authorID, articleID, period string) (models.ArticleStats, error) {
	if _, err := uuid.Parse(articleID); err != nil {
		return models.ArticleStats{}, &ValidationError{message: "invalid article ID"}
	}
	to := service.now().UTC().Truncate(time.Microsecond)
	if _, _, _, err := statsPeriod(period, to); err != nil {
		return models.ArticleStats{}, err
	}
	article, err := service.repository.ArticleMetadata(ctx, articleID)
	if err != nil {
		return models.ArticleStats{}, fmt.Errorf("get article metadata: %w", err)
	}
	if article == nil || article.Deleted || article.AuthorID != authorID {
		return models.ArticleStats{}, ErrArticleNotFound
	}
	stats, err := service.stats(ctx, authorID, articleID, period, to)
	if err != nil {
		return models.ArticleStats{}, err
	}
	return models.ArticleStats{
		ArticleID: articleID, Title: article.Title, Tags: article.Tags, AuthorStats: stats,
	}, nil
}

func (service *AuthorStatsService) stats(ctx context.Context, authorID, articleID, period string, to time.Time) (models.AuthorStats, error) {
	from, step, count, err := statsPeriod(period, to)
	if err != nil {
		return models.AuthorStats{}, err
	}
	byDay := step == 24*time.Hour
	stored, err := service.repository.AuthorStats(ctx, authorID, articleID, from, to, byDay)
	if err != nil {
		return models.AuthorStats{}, fmt.Errorf("get author stats: %w", err)
	}
	byStart := make(map[int64]models.StatsCounters, len(stored))
	for _, bucket := range stored {
		byStart[bucket.Start.Unix()] = bucket.StatsCounters
	}
	result := models.AuthorStats{
		Period: period, From: from, To: to, Series: make([]models.StatsBucket, 0, count),
	}
	for i := range count {
		start := from.Add(time.Duration(i) * step)
		bucket := models.StatsBucket{Start: start, StatsCounters: byStart[start.Unix()]}
		result.Series = append(result.Series, bucket)
		result.Totals.Impressions += bucket.Impressions
		result.Totals.Opens += bucket.Opens
		result.Totals.Likes += bucket.Likes
		result.Totals.Unlikes += bucket.Unlikes
		result.Totals.Comments += bucket.Comments
		result.Totals.Score += bucket.Score
	}
	return result, nil
}

func statsPeriod(period string, now time.Time) (time.Time, time.Duration, int, error) {
	switch period {
	case "7d":
		return now.Truncate(time.Hour).Add(-167 * time.Hour), time.Hour, 168, nil
	case "30d":
		return now.Truncate(24*time.Hour).AddDate(0, 0, -29), 24 * time.Hour, 30, nil
	case "90d":
		return now.Truncate(24*time.Hour).AddDate(0, 0, -89), 24 * time.Hour, 90, nil
	default:
		return time.Time{}, 0, 0, &ValidationError{message: "period must be 7d, 30d or 90d"}
	}
}
