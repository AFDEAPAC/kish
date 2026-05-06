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
	ID         string     `bson:"_id"         json:"id"`
	UserID     string     `bson:"user_id"     json:"user_id"`
	TokenHash  string     `bson:"token_hash"  json:"-"`
	ExpiresAt  time.Time  `bson:"expires_at"  json:"expires_at"`
	RevokedAt  *time.Time `bson:"revoked_at"  json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `bson:"created_at"  json:"created_at"`
	LastUsedAt *time.Time `bson:"last_used_at" json:"last_used_at,omitempty"`
}

// IsValid reports whether the session can be used to issue a new access token.
// A session is valid when it has not expired and has not been revoked.
func (s *Session) IsValid(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}
