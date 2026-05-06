package user

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a user lookup finds no matching record.
var ErrNotFound = errors.New("user not found")

// ErrEmailConflict is returned when attempting to create a user with an email
// that already exists in the system.
var ErrEmailConflict = errors.New("email already exists")

// Repository defines the persistence interface for the User domain.
// Implementations must not expose password hashes beyond what is necessary
// for internal authentication operations.
type Repository interface {
	// Create persists a new user and returns the stored record.
	Create(ctx context.Context, u *User) (*User, error)

	// FindByID returns the user with the given ID, or ErrNotFound.
	FindByID(ctx context.Context, id string) (*User, error)

	// FindByEmail returns the user with the given email address, or ErrNotFound.
	// Used exclusively for authentication; the returned user includes PasswordHash.
	FindByEmail(ctx context.Context, email string) (*User, error)

	// Update applies the non-zero fields of input to the user identified by id.
	Update(ctx context.Context, id string, input UpdateInput) (*User, error)

	// List returns all user records, ordered by created_at ascending.
	List(ctx context.Context) ([]*User, error)

	// CountByRole returns the number of users with the given role.
	// Used to enforce the last-admin invariant.
	CountByRole(ctx context.Context, role UserRole) (int64, error)

	// UpdatePasswordHash replaces the stored bcrypt hash for the user.
	// This is the only operation that modifies the password field.
	UpdatePasswordHash(ctx context.Context, id, hash string) error
}
