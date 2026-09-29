package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token lifetimes: short-lived access token, long-lived rotating refresh.
const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
)

// Errors returned by VerifyAccess.
var (
	ErrTokenExpired = errors.New("auth: token expired")
	ErrTokenInvalid = errors.New("auth: token invalid")
)

// TokenService signs and verifies JWT access tokens.
type TokenService struct {
	secret []byte
}

// NewTokenService derives the HS256 signing key from the master key so no
// extra configuration is required.
func NewTokenService(masterKey string) *TokenService {
	sum := sha256.Sum256([]byte("modelhub-jwt:" + masterKey))
	return &TokenService{secret: sum[:]}
}

// AccessClaims is the JWT payload carried by access tokens.
type AccessClaims struct {
	UserID int64  `json:"uid"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// SignAccess mints an access token for the user.
func (s *TokenService) SignAccess(userID int64, email string) (string, error) {
	return s.signWithTTL(userID, email, AccessTokenTTL)
}

func (s *TokenService) signWithTTL(userID int64, email string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := AccessClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "modelhub",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// VerifyAccess parses and validates an access token.
func (s *TokenService) VerifyAccess(tokenString string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	tok, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		return s.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuedAt())
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}
	if !tok.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// NewRefreshToken returns a random opaque refresh credential.
func NewRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: read refresh token: %w", err)
	}
	return "rft_" + hex.EncodeToString(buf), nil
}
