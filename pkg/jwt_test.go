package pkg

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateToken(t *testing.T) {
	t.Setenv("JWTKEY", "test-secret")
	before := time.Now().Add(23*time.Hour + 59*time.Minute)
	tokenString, err := GenerateToken("user-42")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			t.Fatalf("method=%v", token.Method)
		}
		return []byte("test-secret"), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("ParseWithClaims: %v", err)
	}
	claims := token.Claims.(*Claims)
	if claims.UserID != "user-42" {
		t.Fatalf("user_id=%q", claims.UserID)
	}
	if claims.ExpiresAt == nil || claims.ExpiresAt.Time.Before(before) || claims.ExpiresAt.Time.After(time.Now().Add(24*time.Hour+time.Minute)) {
		t.Fatalf("expiration=%v", claims.ExpiresAt)
	}
}
