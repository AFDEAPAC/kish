package clienttoken

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a client token lookup finds no matching record.
var ErrNotFound = errors.New("client token not found")

// Repository defines the persistence interface for the ClientToken domain.
type Repository interface {
	// Create persists a new client token record and returns the stored record.
	Create(ctx context.Context, t *ClientToken) (*ClientToken, error)

	// FindByID returns the client token with the given ID, or ErrNotFound.
	FindByID(ctx context.Context, id string) (*ClientToken, error)

	// FindByTokenHash returns the client token whose token_hash matches hash.
	// Returns ErrNotFound when no match exists.
	// The caller must verify IsValid on the returned token.
	FindByTokenHash(ctx context.Context, hash string) (*ClientToken, error)

	// ListByUserID returns all client tokens owned by the given user,
	// ordered by created_at descending.
	ListByUserID(ctx context.Context, userID string) ([]*ClientToken, error)

	// Revoke marks the client token identified by id as revoked.
	Revoke(ctx context.Context, id string) error
}
