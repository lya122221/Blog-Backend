package repositories

import (
	"analytics/internal/models"
	"context"
	"fmt"
	"time"
)

const selectArticleMetadata = `SELECT
    toString(tupleElement(argMaxMerge(metadata), 1)),
    tupleElement(argMaxMerge(metadata), 2),
    tupleElement(argMaxMerge(metadata), 3),
    tupleElement(argMaxMerge(metadata), 4)
FROM article_state
WHERE article_id = toUUID(?)
GROUP BY article_id`

const activeAuthorArticles = `article_id IN (
    SELECT article_id FROM article_state
    GROUP BY article_id
    HAVING tupleElement(argMaxMerge(metadata), 1) = toUUID(?)
        AND tupleElement(argMaxMerge(metadata), 4) = 0
)`

func (repository *ClickHouse) ArticleMetadata(ctx context.Context, articleID string) (*models.ArticleMetadata, error) {
	rows, err := repository.conn.Query(ctx, selectArticleMetadata, articleID)
	if err != nil {
		return nil, fmt.Errorf("query article metadata: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("read article metadata: %w", err)
		}
		return nil, nil
	}
	var article models.ArticleMetadata
	var deleted uint8
	if err := rows.Scan(&article.AuthorID, &article.Title, &article.Tags, &deleted); err != nil {
		return nil, fmt.Errorf("scan article metadata: %w", err)
	}
	article.Deleted = deleted != 0
	if article.Tags == nil {
		article.Tags = []string{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read article metadata: %w", err)
	}
	return &article, nil
}

func (repository *ClickHouse) AuthorStats(ctx context.Context, authorID, articleID string, from, to time.Time, byDay bool) ([]models.StatsBucket, error) {
	table, bucketExpression := "article_stats_hour", "toStartOfHour(occurred_at)"
	boundary := to.Truncate(time.Hour)
	if byDay {
		table, bucketExpression = "article_stats_day", "toStartOfDay(occurred_at)"
		boundary = to.Truncate(24 * time.Hour)
	}
	articleFilter := ""
	if articleID != "" {
		articleFilter = " AND article_id = toUUID(?)"
	}
	queryAggregates := `SELECT bucket_start, event_type, score_delta, uniqExactMerge(event_ids) AS event_count
FROM ` + table + `
WHERE bucket_start >= ? AND bucket_start < ? AND ` + activeAuthorArticles + articleFilter + `
GROUP BY bucket_start, article_id, event_type, score_delta`
	args := []any{from, boundary, authorID}
	if articleID != "" {
		args = append(args, articleID)
	}
	buckets := make(map[time.Time]*models.StatsBucket)
	if err := repository.collectStats(ctx, queryAggregates, args, buckets); err != nil {
		return nil, fmt.Errorf("query completed stats buckets: %w", err)
	}

	queryCurrent := `SELECT ` + bucketExpression + ` AS bucket_start, event_type, score_delta, uniqExact(event_id) AS event_count
FROM article_events FINAL
WHERE occurred_at >= ? AND occurred_at < ?
    AND event_type IN ('article.impression', 'article.opened', 'article.liked', 'article.unliked', 'comment.created')
    AND ` + activeAuthorArticles + articleFilter + `
GROUP BY bucket_start, article_id, event_type, score_delta`
	args = []any{boundary, to, authorID}
	if articleID != "" {
		args = append(args, articleID)
	}
	if err := repository.collectStats(ctx, queryCurrent, args, buckets); err != nil {
		return nil, fmt.Errorf("query current stats bucket: %w", err)
	}
	result := make([]models.StatsBucket, 0, len(buckets))
	for _, bucket := range buckets {
		result = append(result, *bucket)
	}
	return result, nil
}

func (repository *ClickHouse) collectStats(ctx context.Context, query string, args []any, buckets map[time.Time]*models.StatsBucket) error {
	rows, err := repository.conn.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var start time.Time
		var eventType string
		var delta int16
		var count uint64
		if err := rows.Scan(&start, &eventType, &delta, &count); err != nil {
			return err
		}
		bucket := buckets[start]
		if bucket == nil {
			bucket = &models.StatsBucket{Start: start}
			buckets[start] = bucket
		}
		addStat(&bucket.StatsCounters, eventType, int64(delta), int64(count))
	}
	return rows.Err()
}

func addStat(counters *models.StatsCounters, eventType string, delta, count int64) {
	switch models.EventType(eventType) {
	case models.ArticleImpression:
		counters.Impressions += count
	case models.ArticleOpened:
		counters.Opens += count
	case models.ArticleLiked:
		counters.Likes += count
	case models.ArticleUnliked:
		counters.Unlikes += count
	case models.CommentCreated:
		counters.Comments += count
	}
	counters.Score += delta * count
}
