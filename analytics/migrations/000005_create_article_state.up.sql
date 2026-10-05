CREATE TABLE IF NOT EXISTS article_state (
    article_id UUID,
    metadata AggregateFunction(
        argMax,
        Tuple(UUID, String, Array(String), UInt8),
        Tuple(DateTime64(6, 'UTC'), UInt8, UUID)
    )
)
ENGINE = AggregatingMergeTree
ORDER BY article_id;

CREATE MATERIALIZED VIEW IF NOT EXISTS article_state_mv TO article_state AS
SELECT
    article_id,
    argMaxState(
        tuple(assumeNotNull(author_id), title, tags, toUInt8(event_type = 'article.deleted')),
        tuple(occurred_at, toUInt8(event_type = 'article.deleted'), event_id)
    ) AS metadata
FROM article_events
WHERE event_type IN ('article.created', 'article.updated', 'article.deleted')
GROUP BY article_id;

INSERT INTO article_state
SELECT
    article_id,
    argMaxState(
        tuple(assumeNotNull(author_id), title, tags, toUInt8(event_type = 'article.deleted')),
        tuple(occurred_at, toUInt8(event_type = 'article.deleted'), event_id)
    ) AS metadata
FROM article_events FINAL
WHERE event_type IN ('article.created', 'article.updated', 'article.deleted')
GROUP BY article_id;
