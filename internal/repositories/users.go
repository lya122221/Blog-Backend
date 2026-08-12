package repositories

import (
	"blog/internal/models"
)

func (s *Storage) GetUserData(email string) (string, string, error) {
	var hashedPassword string
	var userID string

	err := s.db.QueryRow(`
	SELECT id, password_hash
	FROM users
	WHERE email = $1
	`, email).Scan(&userID, &hashedPassword)

	return userID, hashedPassword, err
}

func (s *Storage) AddNewUser(user models.User) error {
	_, err := s.db.Exec(`
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
	`, user.Username, user.Email, user.PasswordHash)

	return err
}
