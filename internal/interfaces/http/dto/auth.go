package dto

import "github.com/AFDEAPAC/kish/internal/domain/user"

// LoginRequest is the inbound credential payload for POST /api/auth/login.
// Both fields are required; Password is plaintext over TLS and must not be
// logged.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserInfo is the public-safe projection of a domain user embedded in auth
// responses. It deliberately omits PasswordHash and any field considered
// internal (Status, timestamps); add new fields only after confirming they
// are safe to expose to authenticated clients.
type UserInfo struct {
	ID          string        `json:"id"`
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name"`
	Role        user.UserRole `json:"role"`
}

// LoginResponse is the outbound payload for POST /api/auth/login.
//
// AccessToken is a short-lived JWT and TokenType is always "Bearer".
// RefreshToken is a long-lived opaque credential returned exactly once;
// clients must store it securely and treat it as a password equivalent.
// ExpiresIn is the access-token lifetime hint in seconds; the authoritative
// expiry is the exp claim inside AccessToken.
type LoginResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	TokenType    string   `json:"token_type"`
	ExpiresIn    int64    `json:"expires_in"`
	User         UserInfo `json:"user"`
}

// RefreshRequest carries the refresh token presented for rotation. The same
// token cannot be reused after a successful refresh: the server revokes it
// regardless of whether the rotated pair is delivered to the client.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshResponse mirrors LoginResponse without the user profile. The same
// security rules apply: RefreshToken is shown exactly once and ExpiresIn is
// a hint that may diverge from the JWT exp claim if the deployment shortens
// the access-token TTL between releases.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// LogoutRequest carries the refresh token to invalidate. Logout is
// idempotent server-side; clients may safely retry.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// MeResponse is the outbound payload of GET /api/auth/me. AuthMethod
// reflects how the caller authenticated this request ("jwt" or
// "client_token") and is sourced from the auth middleware Principal so
// dashboards can hide JWT-only actions when the caller is on a client
// token.
type MeResponse struct {
	ID          string        `json:"id"`
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name"`
	Role        user.UserRole `json:"role"`
	AuthMethod  string        `json:"auth_method"`
}
