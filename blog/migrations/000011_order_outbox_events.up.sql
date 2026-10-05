ALTER TABLE outbox_events
    ADD COLUMN position BIGINT GENERATED ALWAYS AS IDENTITY;

DROP INDEX idx_outbox_events_pending;

CREATE INDEX idx_outbox_events_pending
    ON outbox_events (position)
    WHERE published_at IS NULL;

CREATE INDEX idx_outbox_events_pending_article
    ON outbox_events (article_id, position)
    WHERE published_at IS NULL;

CREATE INDEX idx_outbox_events_published_at
    ON outbox_events (published_at)
    WHERE published_at IS NOT NULL;
