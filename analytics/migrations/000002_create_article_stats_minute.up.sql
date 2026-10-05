CREATE TABLE IF NOT EXISTS article_stats_minute (
    bucket_start DateTime('UTC'),
    article_id UUID,
    event_type LowCardinality(String),
    score_delta Int16,
    event_ids AggregateFunction(uniqExact, UUID)
)
ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMMDD(bucket_start)
ORDER BY (bucket_start, article_id, event_type, score_delta)
TTL bucket_start + INTERVAL 2 DAY DELETE;

CREATE MATERIALIZED VIEW IF NOT EXISTS article_stats_minute_mv TO article_stats_minute AS
SELECT
    toStartOfMinute(occurred_at) AS bucket_start,
    article_id,
    event_type,
    score_delta,
    uniqExactState(event_id) AS event_ids
FROM article_events
WHERE event_type IN ('article.impression', 'article.opened', 'article.liked', 'article.unliked', 'comment.created')
GROUP BY bucket_start, article_id, event_type, score_delta;

INSERT INTO article_stats_minute
SELECT
    toStartOfMinute(occurred_at) AS bucket_start,
    article_id,
    event_type,
    score_delta,
    uniqExactState(event_id) AS event_ids
FROM article_events FINAL
WHERE event_type IN ('article.impression', 'article.opened', 'article.liked', 'article.unliked', 'comment.created')
GROUP BY bucket_start, article_id, event_type, score_delta;
