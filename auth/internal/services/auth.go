package services

import (
	"auth/internal/models"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type UserRepository interface {
	AddNewUser(user models.User) error
	GetUserByEmail(email string) (models.User, error)
}

type TokenIssuer interface {
	GenerateToken(userID, username string) (string, error)
}

type UserService struct {
	repo   UserRepository
	issuer TokenIssuer
}

func NewUserService(repo UserRepository, issuer TokenIssuer) *UserService {
	return &UserService{repo: repo, issuer: issuer}
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

func (s *UserService) Login(user *models.UserLogin) (string, error) {
	storedUser, err := s.repo.GetUserByEmail(user.Email)
	if err != nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedUser.PasswordHash), []byte(user.Password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return "", models.ErrInvalidCredentials
		}
		return "", err
	}

	return s.issuer.GenerateToken(storedUser.ID, storedUser.Username)
}
