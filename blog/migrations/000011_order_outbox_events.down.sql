DROP INDEX idx_outbox_events_published_at;
DROP INDEX idx_outbox_events_pending_article;
DROP INDEX idx_outbox_events_pending;

ALTER TABLE outbox_events
    DROP COLUMN position;

CREATE INDEX idx_outbox_events_pending
    ON outbox_events (created_at, event_id)
    WHERE published_at IS NULL;
