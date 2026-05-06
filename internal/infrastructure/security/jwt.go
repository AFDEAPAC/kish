package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/AFDEAPAC/kish/internal/domain/user"
)

// ErrTokenExpired is returned when a JWT access token has passed its expiry time.
var ErrTokenExpired = errors.New("token expired")

// ErrTokenInvalid is returned when a token cannot be parsed or its signature
// is not valid for the configured secret.
var ErrTokenInvalid = errors.New("invalid token")

// JWTClaims are the custom claims embedded in the access token.
type JWTClaims struct {
	UserID string      `json:"uid"`
	Role   user.UserRole `json:"role"`
	jwt.RegisteredClaims
}

// JWTService handles signing and verification of JWT access tokens.
type JWTService struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTService creates a JWTService using the given secret and access-token TTL.
func NewJWTService(secret string, ttl time.Duration) *JWTService {
	return &JWTService{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// Issue creates and signs a new JWT access token for the given user.
func (s *JWTService) Issue(userID string, role user.UserRole) (string, error) {
	now := time.Now().UTC()
	claims := JWTClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("jwt sign: %w", err)
	}
	return signed, nil
}

// Verify parses and validates the token string. Returns the embedded claims
// on success, or ErrTokenExpired / ErrTokenInvalid on failure.
func (s *JWTService) Verify(tokenStr string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}
