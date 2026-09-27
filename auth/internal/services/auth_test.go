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

func (r *userRepositoryStub) GetUserByEmail(email string) (models.User, error) {
	return r.user, r.err
}

type tokenIssuerStub struct {
	userID   string
	username string
	err      error
}

func (i *tokenIssuerStub) GenerateToken(userID, username string) (string, error) {
	i.userID = userID
	i.username = username
	return "signed-token", i.err
}

func TestRegister(t *testing.T) {
	repo := &userRepositoryStub{}
	service := NewUserService(repo, nil)
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

func TestLogin(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &userRepositoryStub{user: models.User{ID: "user-123", Username: "alice", PasswordHash: string(hash)}}
	issuer := &tokenIssuerStub{}
	service := NewUserService(repo, issuer)

	token, err := service.Login(&models.UserLogin{Email: "alice@example.com", Password: "secret"})
	if err != nil || token != "signed-token" || issuer.userID != "user-123" || issuer.username != "alice" {
		t.Fatalf("login: token=%q err=%v issuer=%+v", token, err, issuer)
	}

	if _, err := service.Login(&models.UserLogin{Email: "alice@example.com", Password: "wrong"}); !errors.Is(err, models.ErrInvalidCredentials) {
		t.Fatalf("wrong password error: %v", err)
	}

	repo.err = models.ErrInvalidCredentials
	if _, err := service.Login(&models.UserLogin{Email: "missing@example.com", Password: "secret"}); !errors.Is(err, models.ErrInvalidCredentials) {
		t.Fatalf("missing user error: %v", err)
	}

	repo.err = errors.New("database unavailable")
	if _, err := service.Login(&models.UserLogin{Email: "alice@example.com", Password: "secret"}); err == nil || errors.Is(err, models.ErrInvalidCredentials) {
		t.Fatalf("database error: %v", err)
	}

	repo.err = nil
	issuer.err = errors.New("signing failed")
	if _, err := service.Login(&models.UserLogin{Email: "alice@example.com", Password: "secret"}); err == nil {
		t.Fatal("expected signing error")
	}
}
