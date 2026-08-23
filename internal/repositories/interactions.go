package repositories

import (
	"blog/internal/models"

	"github.com/google/uuid"
)

func (s *Storage) GetComments(articleID uuid.UUID) ([]models.Comment, error) {
	rows, err := s.db.Query(`
		SELECT 
			comments.id,
			comments.article_id,
			users.id AS author_id,
			users.username AS author_username,
			comments.content,
			comments.created_at
		FROM comments
		INNER JOIN users 
			ON comments.user_id = users.id
		WHERE comments.article_id = $1
		ORDER BY comments.created_at ASC
	`, articleID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []models.Comment

	for rows.Next() {
		var c models.Comment

		err := rows.Scan(
			&c.ID,
			&c.ArticleID,
			&c.Author.ID,
			&c.Author.Username,
			&c.Content,
			&c.CreatedAt,
		)

		if err != nil {
			return nil, err
		}

		comments = append(comments, c)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return comments, nil
}

func (s *Storage) CreateComment(articleID uuid.UUID, userID string, content string) error {
	_, err := s.db.Exec(`
		INSERT INTO comments (article_id, user_id, content)
		VALUES ($1, $2, $3)
	`, articleID, userID, content)

	return err
}

func (s *Storage) ToggleLike(articleID uuid.UUID, userID string) (bool, int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		DELETE FROM likes
		WHERE article_id = $1 AND user_id = $2
	`, articleID, userID)

	if err != nil {
		return false, 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, 0, err
	}

	liked := true
	if rowsAffected == 0 {
		_, err = tx.Exec(`
			INSERT INTO likes (article_id, user_id)
			VALUES ($1, $2)
		`, articleID, userID)
		if err != nil {
			return false, 0, err
		}
	} else {
		liked = false
	}

	var likesCount int
	err = tx.QueryRow(`
		SELECT COUNT(*)
		FROM likes
		WHERE article_id = $1
	`, articleID).Scan(&likesCount)
	if err != nil {
		return false, 0, err
	}

	if err := tx.Commit(); err != nil {
		return false, 0, err
	}

	return liked, likesCount, nil
}
