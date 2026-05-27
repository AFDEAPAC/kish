// Package user defines the User domain entity, role, and status types.
//
// The User entity represents a persisted authenticated account in Kish.
// Anonymous users (unauthenticated request actors) are not stored as User records.
package user

import "time"

// UserRole classifies a user's capabilities within the platform.
type UserRole string

const (
	// RoleAdmin grants platform-level management capabilities.
	RoleAdmin UserRole = "admin"

	// RoleDeveloper grants normal authenticated user capabilities.
	RoleDeveloper UserRole = "developer"
)

// IsValid reports whether r is a recognised role value.
func (r UserRole) IsValid() bool {
	return r == RoleAdmin || r == RoleDeveloper
}

// UserStatus describes whether a user account is usable.
type UserStatus string

const (
	// StatusActive means the user can authenticate and perform allowed operations.
	StatusActive UserStatus = "active"

	// StatusDisabled means the user cannot log in. Refresh attempts and
	// new client-token authentications are rejected by the auth use case
	// when it re-checks status. Existing JWT access tokens remain valid
	// until their exp claim; the application layer does not maintain a
	// JWT revocation list. Session records are NOT automatically revoked
	// on disable — see application/user.Service.DisableUser for the
	// rationale and the follow-up the admin handler must perform.
	StatusDisabled UserStatus = "disabled"
)

// IsValid reports whether s is a recognised status value.
func (s UserStatus) IsValid() bool {
	return s == StatusActive || s == StatusDisabled
}

// User is the core identity entity for authenticated accounts in Kish.
//
// The password_hash field must never be included in API responses or logs.
// It is populated only when reading from the persistence layer and must
// be cleared before returning a User to the interface layer.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name"`
	Role         UserRole   `json:"role"`
	Status       UserStatus `json:"status"`
	PasswordHash string     `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateInput is the domain-layer payload for creating a new User. The
// caller (typically the application user service) is responsible for
// hashing the plaintext password before populating PasswordHash; the
// domain never sees raw passwords. Role must be a value accepted by
// UserRole.IsValid.
type CreateInput struct {
	Email        string
	DisplayName  string
	Role         UserRole
	PasswordHash string
}

// UpdateInput is the partial-update payload accepted by Repository.Update.
// Zero values mean "leave the existing field alone"; callers cannot clear
// a field through UpdateInput. PasswordHash is intentionally absent so the
// only path that changes the password is Repository.UpdatePasswordHash.
type UpdateInput struct {
	DisplayName string
	Role        UserRole
	Status      UserStatus
}
