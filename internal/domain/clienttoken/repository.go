package clienttoken

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a client-token lookup finds no matching
// record. Callers must treat it as a domain miss, not an infrastructure
// failure.
var ErrNotFound = errors.New("client token not found")

// Repository persists ClientToken records for the client-token application
// service.
//
// Implementations live in the infrastructure layer (currently MongoDB) and
// must be safe for concurrent use. Each method is a single-document
// operation; cross-record consistency is the caller's responsibility. The
// implementation is also responsible for indexing TokenHash and UserID so
// FindByTokenHash and ListByUserID stay cheap under load.
//
// Storage rules (see package doc): TokenHash is always persisted,
// EncryptedToken is optional, and the raw token is never persisted by any
// implementation.
type Repository interface {
	// Create persists a new client token and returns the stored record with
	// ID, CreatedAt, and UpdatedAt populated.
	Create(ctx context.Context, t *ClientToken) (*ClientToken, error)

	// FindByID returns the client token with the given ID, or ErrNotFound.
	// Revoked or expired tokens are still returned and the caller is
	// expected to apply ownership and validity checks.
	FindByID(ctx context.Context, id string) (*ClientToken, error)

	// FindByTokenHash returns the client token whose TokenHash matches
	// hash, or ErrNotFound. Revoked or expired tokens are still returned;
	// the application layer calls IsValid before treating the token as a
	// usable credential.
	FindByTokenHash(ctx context.Context, hash string) (*ClientToken, error)

	// ListByUserID returns every client token owned by userID ordered by
	// created_at descending. Both active and revoked tokens are included so
	// the dashboard can show full history; callers are responsible for
	// hiding sensitive fields (TokenHash, EncryptedToken).
	ListByUserID(ctx context.Context, userID string) ([]*ClientToken, error)

	// Revoke marks the client token identified by id as revoked. Revoke
	// returns ErrNotFound when no such token exists so the application
	// layer can distinguish "already gone" from a transport failure;
	// callers should treat a follow-up not-found as a success.
	Revoke(ctx context.Context, id string) error

	// TouchLastUsed records a successful client-token authentication time.
	// Implementations should treat this as best-effort observability;
	// failure must not invalidate the token at the call site (the
	// application layer ignores the returned error).
	TouchLastUsed(ctx context.Context, id string, usedAt time.Time) error
}
