package middleware

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type JWTClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func AuthMiddleware(encodedPublicKey string) (gin.HandlerFunc, error) {
	key, err := base64.StdEncoding.DecodeString(encodedPublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("ANALYTICS_JWT_PUBLIC_KEY must be a base64-encoded %d-byte Ed25519 public key", ed25519.PublicKeySize)
	}
	publicKey := ed25519.PublicKey(key)

	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is missing"})
			return
		}
		parts := strings.Split(header, " ")
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid auth header format"})
			return
		}
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(parts[1], claims, func(*jwt.Token) (any, error) {
			return publicKey, nil
		},
			jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
			jwt.WithIssuer("blog-auth"),
			jwt.WithAudience("blog"),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
		)
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}
		if claims.UserID == "" || claims.Username == "" || claims.Subject != claims.UserID || claims.IssuedAt == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			return
		}
		userID, err := uuid.Parse(claims.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			return
		}
		c.Set("userID", userID.String())
		c.Set("username", claims.Username)
		c.Next()
	}, nil
}
