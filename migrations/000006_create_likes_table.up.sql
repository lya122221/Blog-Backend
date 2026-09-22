CREATE TABLE IF NOT EXISTS likes (
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (article_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_likes_user_id_article_id
    ON likes (user_id, article_id);
