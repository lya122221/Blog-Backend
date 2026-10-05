package repositories

import (
	"analytics/internal/models"
	"context"
	"fmt"
	"time"
)

const selectPopularArticles = `SELECT toString(r.article_id), s.title, s.tags, r.score
FROM (
    SELECT article_id, sum(score) AS score
    FROM (
        SELECT article_id, sum(toInt64(event_count) * toInt64(score_delta)) AS score
        FROM (
            SELECT article_id, event_type, score_delta, uniqExactMerge(event_ids) AS event_count
            FROM article_stats_minute
            WHERE bucket_start >= ? AND bucket_start < ?
            GROUP BY article_id, event_type, score_delta
        )
        GROUP BY article_id
        UNION ALL
        SELECT article_id, sum(toInt64(score_delta)) AS score
        FROM article_events FINAL
        WHERE event_type IN ('article.impression', 'article.opened', 'article.liked', 'article.unliked', 'comment.created')
            AND ((occurred_at >= ? AND occurred_at < ?) OR (occurred_at >= ? AND occurred_at < ?))
        GROUP BY article_id
    )
    GROUP BY article_id
) AS r
INNER JOIN (
    SELECT article_id,
        tupleElement(argMaxMerge(metadata), 2) AS title,
        tupleElement(argMaxMerge(metadata), 3) AS tags,
        tupleElement(argMaxMerge(metadata), 4) AS deleted
    FROM article_state
    GROUP BY article_id
) AS s ON r.article_id = s.article_id
WHERE s.deleted = 0
ORDER BY r.score DESC, r.article_id ASC
LIMIT ?`

func (repository *ClickHouse) ListPopular(ctx context.Context, since, until time.Time, limit int) ([]models.PopularArticle, error) {
	fullStart := since.Truncate(time.Minute)
	if fullStart.Before(since) {
		fullStart = fullStart.Add(time.Minute)
	}
	fullEnd := until.Truncate(time.Minute)
	rows, err := repository.conn.Query(ctx, selectPopularArticles,
		fullStart, fullEnd, since, fullStart, fullEnd, until, limit)
	if err != nil {
		return nil, fmt.Errorf("query popular articles: %w", err)
	}
	defer rows.Close()

	articles := make([]models.PopularArticle, 0)
	for rows.Next() {
		var article models.PopularArticle
		if err := rows.Scan(&article.ArticleID, &article.Title, &article.Tags, &article.Score); err != nil {
			return nil, fmt.Errorf("scan popular article: %w", err)
		}
		if article.Tags == nil {
			article.Tags = []string{}
		}
		articles = append(articles, article)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read popular articles: %w", err)
	}
	return articles, nil
}
