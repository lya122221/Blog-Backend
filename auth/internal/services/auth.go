package services

import (
	"auth/internal/models"

	"golang.org/x/crypto/bcrypt"
)

type UserRepository interface {
	AddNewUser(user models.User) error
}

type UserService struct {
	repo UserRepository
}

func NewUserService(repo UserRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) Register(user *models.UserRegister) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	u := models.User{
		Email:        user.Email,
		Username:     user.Username,
		PasswordHash: string(hashedPassword),
	}

	return s.repo.AddNewUser(u)
}
