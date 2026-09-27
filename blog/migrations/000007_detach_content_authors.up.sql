BEGIN;

ALTER TABLE articles
    ADD COLUMN author_username VARCHAR(50);

UPDATE articles AS a
SET author_username = u.username
FROM users AS u
WHERE a.author_id = u.id;

ALTER TABLE comments
    ADD COLUMN author_username VARCHAR(50);

UPDATE comments AS c
SET author_username = u.username
FROM users AS u
WHERE c.user_id = u.id;

ALTER TABLE articles
    DROP CONSTRAINT articles_author_id_fkey;

ALTER TABLE comments
    DROP CONSTRAINT comments_user_id_fkey;

ALTER TABLE likes
    DROP CONSTRAINT likes_user_id_fkey;

COMMIT;
