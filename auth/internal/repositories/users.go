package repositories

import (
	"auth/internal/models"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Storage) GetUserByEmail(email string) (models.User, error) {
	var user models.User
	err := s.db.QueryRow(`
		SELECT id, username, password_hash
		FROM users
		WHERE email = $1
	`, email).Scan(&user.ID, &user.Username, &user.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, models.ErrInvalidCredentials
	}

	return user, err
}

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
