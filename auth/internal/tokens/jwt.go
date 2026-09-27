package tokens

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Issuer struct {
	privateKey ed25519.PrivateKey
}

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func NewIssuer(encodedSeed string) (*Issuer, error) {
	seed, err := base64.StdEncoding.DecodeString(encodedSeed)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("AUTH_JWT_PRIVATE_KEY must be a base64-encoded %d-byte Ed25519 seed", ed25519.SeedSize)
	}

	return &Issuer{privateKey: ed25519.NewKeyFromSeed(seed)}, nil
}

func (i *Issuer) GenerateToken(userID, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "blog-auth",
			Subject:   userID,
			Audience:  jwt.ClaimStrings{"blog"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(i.privateKey)
}
