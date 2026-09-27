package repositories

import (
	"auth/internal/models"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Storage) AddNewUser(user models.User) error {
	_, err := s.db.Exec(`
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
	`, user.Username, user.Email, user.PasswordHash)
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errors.Join(models.ErrUserExists, err)
	}

	return err
}
