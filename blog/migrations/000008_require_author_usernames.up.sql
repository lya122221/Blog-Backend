BEGIN;

UPDATE articles AS a
SET author_username = u.username
FROM users AS u
WHERE a.author_username IS NULL AND a.author_id = u.id;

UPDATE comments AS c
SET author_username = u.username
FROM users AS u
WHERE c.author_username IS NULL AND c.user_id = u.id;

ALTER TABLE articles
    ALTER COLUMN author_username SET NOT NULL;

ALTER TABLE comments
    ALTER COLUMN author_username SET NOT NULL;

COMMIT;
