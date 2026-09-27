package handlers

import (
	"auth/internal/models"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type userServiceStub struct {
	user  *models.UserRegister
	login *models.UserLogin
	err   error
}

func (s *userServiceStub) Register(user *models.UserRegister) error {
	s.user = user
	return s.err
}

func (s *userServiceStub) Login(user *models.UserLogin) (string, error) {
	s.login = user
	return "signed-token", s.err
}

func TestRegisterUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &userServiceStub{}
	handler := NewUserHandler(service)
	router := gin.New()
	router.POST("/api/v1/auth/register", handler.RegisterUser)
	path := "/api/v1/auth/register"
	validBody := `{"email":"alice@example.com","username":"alice","password":"secret"}`

	request := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	w := request(validBody)
	if w.Code != http.StatusCreated || w.Body.String() != "null" {
		t.Fatalf("registration response: status=%d body=%s", w.Code, w.Body.String())
	}
	if service.user == nil || service.user.Email != "alice@example.com" || service.user.Username != "alice" || service.user.Password != "secret" {
		t.Fatalf("unexpected user: %+v", service.user)
	}

	for _, body := range []string{`{`, `{"email":"invalid","username":"alice","password":"secret"}`, `{"email":"alice@example.com","username":"alice"}`} {
		w = request(body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid body %q: status=%d body=%s", body, w.Code, w.Body.String())
		}
	}

	service.err = models.ErrUserExists
	w = request(validBody)
	if w.Code != http.StatusConflict || w.Body.String() != `{"error":"User already exists"}` {
		t.Fatalf("duplicate user response: status=%d body=%s", w.Code, w.Body.String())
	}

	service.err = errors.New("database unavailable")
	w = request(validBody)
	if w.Code != http.StatusInternalServerError || w.Body.String() != `{"error":"Failed to create user"}` {
		t.Fatalf("internal error response: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLoginUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &userServiceStub{}
	handler := NewUserHandler(service)
	router := gin.New()
	router.POST("/api/v1/auth/login", handler.LoginUser)
	path := "/api/v1/auth/login"
	validBody := `{"email":"alice@example.com","password":"secret"}`

	request := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	w := request(validBody)
	if w.Code != http.StatusOK || w.Body.String() != `{"token":"signed-token"}` {
		t.Fatalf("login response: status=%d body=%s", w.Code, w.Body.String())
	}
	if service.login == nil || service.login.Email != "alice@example.com" || service.login.Password != "secret" {
		t.Fatalf("unexpected login: %+v", service.login)
	}

	for _, body := range []string{`{`, `{"email":"invalid","password":"secret"}`, `{"email":"alice@example.com"}`} {
		w = request(body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid body %q: status=%d body=%s", body, w.Code, w.Body.String())
		}
	}

	service.err = models.ErrInvalidCredentials
	w = request(validBody)
	if w.Code != http.StatusUnauthorized || w.Body.String() != `{"error":"Invalid email or password"}` {
		t.Fatalf("wrong credentials response: status=%d body=%s", w.Code, w.Body.String())
	}

	service.err = errors.New("database unavailable")
	w = request(validBody)
	if w.Code != http.StatusInternalServerError || w.Body.String() != `{"error":"Failed to log in"}` {
		t.Fatalf("internal error response: status=%d body=%s", w.Code, w.Body.String())
	}
}
