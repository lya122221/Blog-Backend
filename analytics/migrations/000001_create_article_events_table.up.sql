CREATE TABLE IF NOT EXISTS article_events (
    event_id UUID,
    event_type LowCardinality(String),
    article_id UUID,
    author_id Nullable(UUID),
    user_id Nullable(UUID),
    visitor_id Nullable(UUID),
    occurred_at DateTime64(6, 'UTC'),
    score_delta Int16,
    title String,
    tags Array(String),
    ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree
PARTITION BY toYYYYMMDD(occurred_at)
ORDER BY (occurred_at, article_id, event_id)
TTL occurred_at + INTERVAL 30 DAY DELETE;
