package services

import (
	"blog/internal/models"
	"blog/pkg"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type UserRepository interface {
	GetStoredPassword(email string) (string, error)
	AddNewUser(user models.User) error
}

type UserService struct {
	repo UserRepository
}

func NewUserService(repo UserRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) Register(user *models.UserRegister) error {
	hashedPassword, err := GenerateHashedPassword(user.Password)
	if err != nil {
		return err
	}

	u := models.User{
		Email:        user.Email,
		Username:     user.Username,
		PasswordHash: hashedPassword,
	}

	return s.repo.AddNewUser(u)
}

func (s *UserService) Login(user *models.UserLogin) (string, error) {
	hashedPassword, err := s.repo.GetStoredPassword(user.Email)
	if err != nil {
		return "", err
	}

	correctPassword := ComparePasswords(hashedPassword, user.Password)
	if !correctPassword {
		return "", errors.New("Incorrect password")
	}

	token, err := pkg.GenerateToken(user.Email)
	if err != nil {
		return "", err
	}

	return token, nil
}

func GenerateHashedPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(password), bcrypt.DefaultCost,
	)

	if err != nil {
		return "", err
	}

	return string(hashedPassword), nil
}

func ComparePasswords(hashedPassword string, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		return false
	}

	return true
}
