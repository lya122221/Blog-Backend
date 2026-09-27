package tokens

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateToken(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	issuer, err := NewIssuer(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.GenerateToken("user-123", "alice")
	if err != nil {
		t.Fatal(err)
	}
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return ed25519.NewKeyFromSeed(seed).Public(), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}), jwt.WithIssuer("blog-auth"), jwt.WithAudience("blog"))
	if err != nil || !parsed.Valid {
		t.Fatalf("invalid signed token: %v", err)
	}
	if claims.UserID != "user-123" || claims.Username != "alice" || claims.Subject != "user-123" {
		t.Fatalf("invalid claims: %+v", claims)
	}
	if claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) != 24*time.Hour {
		t.Fatalf("unexpected token lifetime: %v", claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time))
	}
}

func TestNewIssuerRejectsInvalidKey(t *testing.T) {
	for _, encoded := range []string{"", "not base64", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := NewIssuer(encoded); err == nil {
			t.Fatalf("accepted invalid seed %q", encoded)
		}
	}
}
