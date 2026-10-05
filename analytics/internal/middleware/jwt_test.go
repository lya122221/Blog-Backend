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

func TestAuthMiddlewareAcceptsBlogTokenAndRejectsInvalidTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	auth, err := AuthMiddleware(base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)))
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/private", auth, func(c *gin.Context) { c.String(http.StatusOK, c.GetString("userID")) })
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": "55555555-5555-4555-8555-555555555555", "username": "alice",
		"sub": "55555555-5555-4555-8555-555555555555", "iss": "blog-auth", "aud": "blog",
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
	sign := func(method jwt.SigningMethod, values jwt.MapClaims, key any) string {
		t.Helper()
		token, err := jwt.NewWithClaims(method, values).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	valid := sign(jwt.SigningMethodEdDSA, claims, privateKey)
	otherSeed := make([]byte, ed25519.SeedSize)
	otherSeed[0] = 1
	expiredClaims := jwt.MapClaims{}
	for key, value := range claims {
		expiredClaims[key] = value
	}
	expiredClaims["exp"] = now.Add(-time.Hour).Unix()
	request := func(header string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if response := request("Bearer " + valid); response.Code != http.StatusOK || response.Body.String() != claims["user_id"] {
		t.Fatalf("valid token response = %d %s", response.Code, response.Body.String())
	}
	for _, header := range []string{
		"", "Token " + valid,
		"Bearer " + sign(jwt.SigningMethodEdDSA, claims, ed25519.NewKeyFromSeed(otherSeed)),
		"Bearer " + sign(jwt.SigningMethodHS256, claims, []byte("secret")),
		"Bearer " + sign(jwt.SigningMethodEdDSA, expiredClaims, privateKey),
		"Bearer " + sign(jwt.SigningMethodEdDSA, jwt.MapClaims{
			"user_id": "not-a-uuid", "username": "alice", "sub": "not-a-uuid",
			"iss": "blog-auth", "aud": "blog", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		}, privateKey),
	} {
		if response := request(header); response.Code != http.StatusUnauthorized {
			t.Fatalf("header = %q, response = %d", header, response.Code)
		}
	}
}

func TestAuthMiddlewareRejectsInvalidPublicKey(t *testing.T) {
	for _, value := range []string{"", "invalid", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if middleware, err := AuthMiddleware(value); err == nil || middleware != nil {
			t.Fatalf("key %q: middleware=%v error=%v", value, middleware, err)
		}
	}
}
