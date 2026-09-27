package middleware

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func signedToken(t *testing.T, method jwt.SigningMethod, claims jwt.Claims, key any) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	encodedPublicKey := base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey))
	middleware, err := AuthMiddleware(encodedPublicKey)
	if err != nil {
		t.Fatal(err)
	}

	request := func(header string) *httptest.ResponseRecorder {
		t.Helper()
		router := gin.New()
		router.GET("/private", middleware, func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"user_id": c.GetString("userID"), "username": c.GetString("username")})
		})
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	now := time.Now()
	validClaims := jwt.MapClaims{
		"user_id": "user-123", "username": "alice", "sub": "user-123",
		"iss": "blog-auth", "aud": "blog", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
	valid := signedToken(t, jwt.SigningMethodEdDSA, validClaims, privateKey)
	w := request("Bearer " + valid)
	if w.Code != http.StatusOK || w.Body.String() != `{"user_id":"user-123","username":"alice"}` {
		t.Fatalf("valid token response: status=%d body=%s", w.Code, w.Body.String())
	}

	otherKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"missing header", "", http.StatusUnauthorized},
		{"bad header", "Token " + valid, http.StatusUnauthorized},
		{"empty token", "Bearer ", http.StatusUnauthorized},
		{"bad signature", "Bearer " + signedToken(t, jwt.SigningMethodEdDSA, validClaims, otherKey), http.StatusUnauthorized},
		{"old algorithm", "Bearer " + signedToken(t, jwt.SigningMethodHS256, validClaims, []byte("secret")), http.StatusUnauthorized},
		{"expired", "Bearer " + signedToken(t, jwt.SigningMethodEdDSA, claimsWith(validClaims, "exp", now.Add(-time.Hour).Unix()), privateKey), http.StatusUnauthorized},
		{"wrong issuer", "Bearer " + signedToken(t, jwt.SigningMethodEdDSA, claimsWith(validClaims, "iss", "other"), privateKey), http.StatusUnauthorized},
		{"wrong audience", "Bearer " + signedToken(t, jwt.SigningMethodEdDSA, claimsWith(validClaims, "aud", "other"), privateKey), http.StatusUnauthorized},
		{"missing user id", "Bearer " + signedToken(t, jwt.SigningMethodEdDSA, claimsWith(validClaims, "user_id", ""), privateKey), http.StatusUnauthorized},
		{"missing username", "Bearer " + signedToken(t, jwt.SigningMethodEdDSA, claimsWith(validClaims, "username", ""), privateKey), http.StatusUnauthorized},
		{"different subject", "Bearer " + signedToken(t, jwt.SigningMethodEdDSA, claimsWith(validClaims, "sub", "other"), privateKey), http.StatusUnauthorized},
		{"missing issued at", "Bearer " + signedToken(t, jwt.SigningMethodEdDSA, claimsWithout(validClaims, "iat"), privateKey), http.StatusUnauthorized},
		{"missing expiration", "Bearer " + signedToken(t, jwt.SigningMethodEdDSA, claimsWithout(validClaims, "exp"), privateKey), http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := request(tc.header)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestAuthMiddlewareInvalidKey(t *testing.T) {
	for _, key := range []string{"", "not base64", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if middleware, err := AuthMiddleware(key); err == nil || middleware != nil {
			t.Fatalf("invalid key %q: middleware=%v err=%v", key, middleware, err)
		}
	}
}

func claimsWith(original jwt.MapClaims, key string, value any) jwt.MapClaims {
	claims := claimsWithout(original, key)
	claims[key] = value
	return claims
}

func claimsWithout(original jwt.MapClaims, key string) jwt.MapClaims {
	claims := make(jwt.MapClaims, len(original))
	for name, value := range original {
		if name != key {
			claims[name] = value
		}
	}
	return claims
}
