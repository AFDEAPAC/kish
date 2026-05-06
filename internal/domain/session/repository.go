package session

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a session lookup finds no matching record.
var ErrNotFound = errors.New("session not found")

// Repository defines the persistence interface for the Session (refresh token) domain.
type Repository interface {
	// Create persists a new session record.
	Create(ctx context.Context, s *Session) (*Session, error)

	// FindByTokenHash returns the session whose token_hash matches hash, or ErrNotFound.
	// The caller must verify IsValid on the returned session.
	FindByTokenHash(ctx context.Context, hash string) (*Session, error)

	// Revoke marks the session identified by id as revoked by setting revoked_at to now.
	Revoke(ctx context.Context, id string) error

	// RevokeAllByUserID revokes all active sessions belonging to userID.
	// Called when a user is disabled or when a full logout is requested.
	RevokeAllByUserID(ctx context.Context, userID string) error
}
