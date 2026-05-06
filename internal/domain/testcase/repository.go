package testcase

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Repository implementations when the requested
// TestCase does not exist in the backing store.
var ErrNotFound = errors.New("testcase not found")

// Repository is the persistence interface for TestCase entities.
//
// Implementations live in the infrastructure layer (e.g. infrastructure/mongodb).
// The domain and application layers depend only on this interface, keeping them
// free of any database-specific code.
type Repository interface {
	// Create persists a new TestCase. The TestCase ID must already be set by
	// the caller (application layer generates it before calling Create).
	Create(ctx context.Context, tc *TestCase) error

	// Update replaces the environment, artifacts, and metadata of an existing
	// TestCase identified by id. CreatedAt must be preserved by the implementation.
	// Returns ErrNotFound if the id does not exist.
	Update(ctx context.Context, id string, tc *TestCase) error

	// FindByID retrieves a TestCase by its ID.
	// Returns ErrNotFound if the id does not exist.
	FindByID(ctx context.Context, id string) (*TestCase, error)
}
