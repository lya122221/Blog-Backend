package services

import (
	"auth/internal/models"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

type userRepositoryStub struct {
	user models.User
	err  error
}

func (r *userRepositoryStub) AddNewUser(user models.User) error {
	r.user = user
	return r.err
}

func TestRegister(t *testing.T) {
	repo := &userRepositoryStub{}
	service := NewUserService(repo)
	user := &models.UserRegister{
		Email:    "alice@example.com",
		Username: "alice",
		Password: "secret",
	}

	if err := service.Register(user); err != nil {
		t.Fatal(err)
	}
	if repo.user.Email != user.Email || repo.user.Username != user.Username {
		t.Fatalf("unexpected user: %+v", repo.user)
	}
	if repo.user.PasswordHash == user.Password {
		t.Fatal("password was stored without hashing")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.user.PasswordHash), []byte(user.Password)); err != nil {
		t.Fatalf("invalid password hash: %v", err)
	}

	repo.err = models.ErrUserExists
	if err := service.Register(user); !errors.Is(err, models.ErrUserExists) {
		t.Fatalf("duplicate user error: %v", err)
	}
}
