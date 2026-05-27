package user

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a user lookup finds no matching record.
// Callers must treat it as a domain miss, not an infrastructure failure.
var ErrNotFound = errors.New("user not found")

// ErrEmailConflict is returned when Create would violate the email-unique
// invariant. The infrastructure layer is responsible for translating its
// native unique-index errors into this sentinel.
var ErrEmailConflict = errors.New("email already exists")

// Repository persists User aggregates for the user-management and auth use
// cases.
//
// Implementations live in the infrastructure layer (currently MongoDB) and
// must be safe for concurrent use. Methods are single-document operations
// and are not wrapped in a multi-document transaction; callers that need to
// coordinate multiple changes (e.g. disable user + revoke sessions) must
// orchestrate them at the application layer.
//
// PasswordHash is loaded from storage only for the auth use case
// (FindByEmail / FindByID + ChangePassword). The application service is
// expected to blank PasswordHash before returning users to any
// non-authentication caller.
type Repository interface {
	// Create persists a new user. Returns ErrEmailConflict when another user
	// with the same email already exists. Other errors represent
	// infrastructure failures.
	Create(ctx context.Context, u *User) (*User, error)

	// FindByID returns the user with the given ID, or ErrNotFound when no
	// such user exists. The returned user includes PasswordHash.
	FindByID(ctx context.Context, id string) (*User, error)

	// FindByEmail returns the user with the given email address, or
	// ErrNotFound. Reserved for authentication paths: the returned user
	// includes PasswordHash and must not be forwarded to clients.
	FindByEmail(ctx context.Context, email string) (*User, error)

	// Update applies the non-zero fields of input to the user identified by
	// id. Returns ErrNotFound when the id does not exist. Implementations
	// must refresh updated_at and must not touch PasswordHash; use
	// UpdatePasswordHash for that field.
	Update(ctx context.Context, id string, input UpdateInput) (*User, error)

	// List returns every user, ordered by created_at ascending. Used by
	// admin-only endpoints; the application layer clears PasswordHash on
	// the returned slice before responding.
	List(ctx context.Context) ([]*User, error)

	// CountByRole returns the number of users with the given role and is
	// used by the application layer to enforce the last-admin invariant
	// before disabling or demoting an account.
	CountByRole(ctx context.Context, role UserRole) (int64, error)

	// UpdatePasswordHash replaces the stored password hash for the user.
	// It is the only operation that mutates the password field and the
	// caller is responsible for supplying a value produced by the
	// configured PasswordHasher port.
	UpdatePasswordHash(ctx context.Context, id, hash string) error
}
