package repositories

import (
	"blog/internal/models"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func (s *Storage) GetArticlesWithoutTags(limit int, offset int) (*sql.Rows, error) {
	return s.db.Query(`
		SELECT 
			articles.id, 
			articles.title, 
			articles.content,
			articles.views_count,
			articles.created_at,
			users.id AS author_id,
			users.username AS author_username,
			COALESCE((
        SELECT array_agg(tags.name)
        FROM article_tags at
        INNER JOIN tags 
          ON tags.id = at.tag_id
        WHERE at.article_id = articles.id
    	), '{}') AS all_tags
		FROM articles
		INNER JOIN users 
			ON articles.author_id = users.id 
		ORDER BY articles.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
}

func (s *Storage) GetArticlesWithTags(limit int, offset int, tags []string) (*sql.Rows, error) {
	return s.db.Query(`
		SELECT 
			articles.id, 
			articles.title, 
			articles.content,
			articles.views_count,
			articles.created_at,
			users.id AS author_id,
			users.username AS author_username,
			COALESCE((
        SELECT array_agg(tags.name)
        FROM article_tags at
        INNER JOIN tags 
          ON tags.id = at.tag_id
        WHERE at.article_id = articles.id
    	), '{}') AS all_tags
		FROM articles
		INNER JOIN users
			ON articles.author_id = users.id 
		INNER JOIN article_tags
			ON article_tags.article_id = articles.id
		INNER JOIN tags
			ON tags.id = article_tags.tag_id
		WHERE tags.name = ANY($1)
		GROUP BY articles.id, users.id
		ORDER BY articles.created_at DESC
		LIMIT $2 OFFSET $3
	`, tags, limit, offset)
}

func (s *Storage) GetArticles(limit int, offset int, tags []string) ([]models.Article, error) {
	var rows *sql.Rows
	var err error

	if len(tags) == 0 {
		rows, err = s.GetArticlesWithoutTags(limit, offset)
	} else {
		rows, err = s.GetArticlesWithTags(limit, offset, tags)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []models.Article

	for rows.Next() {
		var a models.Article

		err := rows.Scan(
			&a.ID,
			&a.Title,
			&a.Content,
			&a.ViewsCount,
			&a.CreatedAt,
			&a.Author.ID,
			&a.Author.Username,
			pq.Array(&a.Tags),
		)

		if err != nil {
			return nil, err
		}

		articles = append(articles, a)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return articles, nil
}

func (s *Storage) CreateArticle(authorID, title, content string, tags []string) error {
	tx, err := s.db.Begin()

	if err != nil {
		return err
	}

	defer tx.Rollback()

	var articleID string
	err = tx.QueryRow(`
		INSERT INTO articles (author_id, title, content)
		VALUES ($1, $2, $3)
		RETURNING id
	`, authorID, title, content).Scan(&articleID)

	if err != nil {
		return err
	}

	for _, tag := range tags {
		var tagID string
		err := tx.QueryRow(`
			INSERT INTO tags (name)
			VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name 
			RETURNING id
		`, tag).Scan(&tagID)

		if err != nil {
			return err
		}

		_, err = tx.Exec(`
			INSERT INTO article_tags (article_id, tag_id)
			VALUES ($1, $2)
		`, articleID, tagID)

		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *Storage) GetArticleWithID(articleID uuid.UUID) (*models.Article, error) {
	var article models.Article

	err := s.db.QueryRow(`
		SELECT 
			id,
			title,
			content,
			views_count,
			created_at,
			(SELECT username 
				FROM users 
				WHERE id = articles.author_id
			),
			articles.author_id,
			COALESCE((
				SELECT array_agg(t.name)
				FROM article_tags at
				INNER JOIN tags t 
					ON t.id = at.tag_id
				WHERE at.article_id = articles.id
			), '{}')
		FROM articles
		WHERE id = $1
	`, articleID).Scan(
		&article.ID,
		&article.Title,
		&article.Content,
		&article.ViewsCount,
		&article.CreatedAt,
		&article.Author.Username,
		&article.Author.ID,
		pq.Array(&article.Tags),
	)

	if err != nil {
		return nil, err
	}

	return &article, nil
}

func (s *Storage) IncrementViewsCount(articleID uuid.UUID) error {
	key := fmt.Sprintf("article:%s:views", articleID)

	return s.redis.Incr(context.Background(), key).Err()
}

func (s *Storage) GetAndClearViewsCount() (map[string]int, error) {
	ctx := context.Background()
	viewsToUpdate := make(map[string]int)

	var cursor uint64
	pattern := "article:*:views"

	for {
		var keys []string
		var err error

		keys, cursor, err = s.redis.Scan(ctx, cursor, pattern, 50).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			val, err := s.redis.GetDel(ctx, key).Result()
			if err != nil {
				if err == redis.Nil {
					continue
				}

				continue
			}

			viewsCount, err := strconv.Atoi(val)
			if err != nil || viewsCount <= 0 {
				continue
			}

			parts := strings.Split(key, ":")
			if len(parts) == 3 {
				articleID := parts[1]

				viewsToUpdate[articleID] += viewsCount
			}
		}

		if cursor == 0 {
			break
		}
	}

	return viewsToUpdate, nil
}

func (s *Storage) UpdateArticleViews(viewsCount int, articleID uuid.UUID) error {
	_, err := s.db.Exec(`
		UPDATE articles
		SET views_count = views_count + $1
		WHERE id = $2
	`, viewsCount, articleID)

	if err != nil {
		return err
	}

	return nil
}

func (s *Storage) UpdateArticle(authorID string, articleID uuid.UUID, request models.UpdateArticleRequest) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var storedAuthorID string
	err = tx.QueryRow(`
		SELECT author_id
		FROM articles
		WHERE id = $1
	`, articleID).Scan(&storedAuthorID)
	if err != nil {
		return err
	}
	if storedAuthorID != authorID {
		return errors.New("Invalid authorID")
	}

	_, err = tx.Exec(`
		UPDATE articles
		SET title = $1, content = $2
		WHERE id = $3
	`, request.Title, request.Content, articleID)

	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		DELETE FROM article_tags
		WHERE article_id = $1
	`, articleID)

	if err != nil {
		return err
	}

	for _, tag := range request.Tags {
		var tagID string

		err := tx.QueryRow(`
			INSERT INTO tags (name)
			VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, tag).Scan(&tagID)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
			INSERT INTO article_tags (article_id, tag_id) 
			VALUES ($1, $2)
		`, articleID, tagID)

		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *Storage) DeleteArticle(authorID string, articleID uuid.UUID) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var storedAuthorID string
	err = tx.QueryRow(`
		SELECT author_id
		FROM articles
		WHERE id = $1
	`, articleID).Scan(&storedAuthorID)

	if err != nil {
		return err
	}

	if storedAuthorID != authorID {
		return errors.New("Invalid authorID")
	}

	_, err = tx.Exec(`
		DELETE FROM articles
		WHERE id = $1
	`, articleID)

	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}
