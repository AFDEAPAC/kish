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

// Session represents a stored refresh-token record.
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
