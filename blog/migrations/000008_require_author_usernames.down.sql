BEGIN;

ALTER TABLE articles
    ALTER COLUMN author_username DROP NOT NULL;

ALTER TABLE comments
    ALTER COLUMN author_username DROP NOT NULL;

COMMIT;
