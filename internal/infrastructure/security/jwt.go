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

// JWTService signs and verifies short-lived JWT access tokens using HS256.
//
// HS256 is fixed by deployment policy: only one process signs and verifies
// tokens with the shared symmetric secret loaded from auth.jwt_secret in
// config. Switching to RS256 or another algorithm requires updating both
// signing (Issue) and the algorithm check in Verify so a downgrade attack
// cannot be silently accepted. The secret is held in memory only; rotating
// it requires restarting the process, and all outstanding access tokens
// become invalid at that point.
//
// JWTService methods are safe for concurrent use.
type JWTService struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTService wires a JWTService.
//
// secret comes from auth.jwt_secret in config and must be high-entropy. An
// empty or short secret is accepted at construction time but produces tokens
// that anyone with the deployment config can forge; the config loader is
// expected to reject such values for production use.
//
// ttl is the access-token lifetime applied to the exp claim of every token
// issued from this service. Callers (auth.Service) should treat the JWT exp
// claim as authoritative; do not derive the TTL by re-parsing the token.
func NewJWTService(secret string, ttl time.Duration) *JWTService {
	return &JWTService{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// Issue signs a new HS256 JWT for the given user id and role. The IssuedAt
// and ExpiresAt claims are derived from the current UTC clock; the rest of
// the access-control decision (role escalation, status disabled, etc.) is
// the caller's responsibility.
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

// Verify parses tokenStr, checks its signature against the configured
// secret, enforces that the algorithm is HMAC (rejecting "none" and
// asymmetric algorithms to prevent downgrade attacks), and validates exp /
// nbf / iat via the library defaults.
//
// Verify returns ErrTokenExpired only when the underlying library reports
// exp/nbf failure; all other parse, signature, or claim errors are
// collapsed into ErrTokenInvalid so the caller cannot distinguish "wrong
// signature" from "malformed token". Verify is purely CPU-bound and does
// not respect context cancellation.
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
