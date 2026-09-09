package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func signedToken(t *testing.T, method jwt.SigningMethod, claims jwt.MapClaims, key any) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func authRequest(header string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/private", AuthMiddleware(), func(c *gin.Context) { value, _ := c.Get("userID"); c.JSON(http.StatusOK, gin.H{"user_id": value}) })
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthMiddleware(t *testing.T) {
	t.Setenv("JWTKEY", "secret")
	valid := signedToken(t, jwt.SigningMethodHS256, jwt.MapClaims{"user_id": "u1", "exp": time.Now().Add(time.Hour).Unix()}, []byte("secret"))
	tests := []struct {
		name, header string
		want         int
	}{
		{"valid", "Bearer " + valid, 200},
		{"missing", "", 401},
		{"wrong format", "Token " + valid, 401},
		{"extra part", "Bearer " + valid + " extra", 401},
		{"bad signature", "Bearer " + signedToken(t, jwt.SigningMethodHS256, jwt.MapClaims{"user_id": "u1"}, []byte("other")), 401},
		{"expired", "Bearer " + signedToken(t, jwt.SigningMethodHS256, jwt.MapClaims{"user_id": "u1", "exp": time.Now().Add(-time.Hour).Unix()}, []byte("secret")), 401},
		{"missing claim", "Bearer " + signedToken(t, jwt.SigningMethodHS256, jwt.MapClaims{}, []byte("secret")), 401},
		{"wrong claim type", "Bearer " + signedToken(t, jwt.SigningMethodHS256, jwt.MapClaims{"user_id": 1}, []byte("secret")), 401},
		{"wrong algorithm", "Bearer " + signedToken(t, jwt.SigningMethodNone, jwt.MapClaims{"user_id": "u1"}, jwt.UnsafeAllowNoneSignatureType), 401},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := authRequest(tc.header)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
