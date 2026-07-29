package repositories

import (
	"blog/internal/models"
	"database/sql"
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
			(
				SELECT array_agg(tags.name)
				FROM article_tags at
				INNER JOIN tags ON tags.id = at.tag_id
				WHERE at.article_id = articles.id
			) AS all_tags
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
			(
				SELECT array_agg(tags.name)
				FROM article_tags at
				INNER JOIN tags 
					ON tags.id = at.tag_id
				WHERE at.article_id = articles.id
			) AS all_tags
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
	defer rows.Close()

	if err != nil {
		return nil, err
	}

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
			&a.Tags,
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
