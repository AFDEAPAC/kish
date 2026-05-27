// Package session defines the Session (refresh token) domain entity.
//
// A Session represents a persisted refresh token that allows a user to obtain
// new JWT access tokens without re-entering credentials.
//
// Refresh tokens are stored as SHA-256 hashes; the raw token is never persisted.
// Refresh token rotation is applied on each use: the old session is revoked and
// a new one is issued.
package session

import "time"

// Session is a single refresh-token record bound to one user.
//
// State transitions:
//   - Created on successful Login (auth.Service.Login) with ExpiresAt set to
//     now+refreshTTL and RevokedAt nil.
//   - Marked invalid by setting RevokedAt; the record is kept rather than
//     deleted so historical audit and the package-level rotation rule
//     ("old session revoked before new session is observable") remain
//     enforceable.
//   - Treated as invalid once ExpiresAt has passed even without an
//     explicit revocation; cleanup of expired-but-not-revoked rows is left
//     to operators.
//
// TokenHash is the SHA-256 of the raw refresh token and is the only secret
// in the record; it must never be logged or returned in API responses
// (the json:"-" tag is enforcement, not documentation).
type Session struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	TokenHash  string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// IsValid reports whether the session can be used to issue a new access token.
// A session is valid when it has not expired and has not been revoked.
func (s *Session) IsValid(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}
