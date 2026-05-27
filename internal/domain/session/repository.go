package session

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a session lookup finds no matching record.
// Callers must treat it as a domain miss, not an infrastructure failure.
var ErrNotFound = errors.New("session not found")

// Repository persists refresh-token sessions for the authentication use case.
//
// Implementations live in the infrastructure layer (currently MongoDB) and
// must be safe for concurrent use. Each method must respect ctx cancellation
// for any I/O it performs. Sessions are independent records: this
// repository does not provide a multi-document transaction and callers must
// not assume Create+Revoke pairs are atomic.
//
// Session records carry the SHA-256 hash of the refresh token, never the raw
// token. Implementations must not log or return the raw value even if a
// caller mistakenly stores it.
type Repository interface {
	// Create persists a new session and returns the stored record with any
	// implementation-assigned identifier (e.g. Mongo ObjectID) populated.
	// The supplied Session must have UserID, TokenHash, and ExpiresAt set;
	// CreatedAt and ID are filled in by the implementation.
	Create(ctx context.Context, s *Session) (*Session, error)

	// FindByTokenHash returns the session whose TokenHash matches hash.
	// Returns ErrNotFound when no record matches. The lookup is by hash
	// only; an expired or already-revoked record is still returned and the
	// caller must invoke Session.IsValid before honouring it.
	FindByTokenHash(ctx context.Context, hash string) (*Session, error)

	// Revoke marks the session identified by id as revoked at the current
	// time. Revoke is idempotent: calling it on an unknown or already
	// revoked id returns nil so retries and concurrent logouts converge to
	// the same observable state.
	Revoke(ctx context.Context, id string) error

	// RevokeAllByUserID revokes every active session owned by userID. It is
	// used during admin sign-out or hard account disable flows. Implementations
	// must not error when the user has no active sessions.
	RevokeAllByUserID(ctx context.Context, userID string) error
}
