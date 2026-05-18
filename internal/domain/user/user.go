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

	// StatusDisabled means the user cannot log in, use refresh tokens, or use client tokens.
	// All existing sessions are revoked when a user is disabled.
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

// CreateInput carries the fields required to create a new user.
type CreateInput struct {
	Email        string
	DisplayName  string
	Role         UserRole
	PasswordHash string
}

// UpdateInput carries the fields that can be updated on an existing user.
// Zero values are ignored; only non-empty fields are applied.
type UpdateInput struct {
	DisplayName string
	Role        UserRole
	Status      UserStatus
}
