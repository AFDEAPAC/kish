package dto

import (
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/user"
)

// CreateUserRequest is the inbound payload of POST /api/users. Password is
// plaintext over TLS; the application layer hashes it with the configured
// PasswordHasher before persistence. Email uniqueness is enforced by the
// repository's email index and surfaces as user.ErrEmailConflict (409).
type CreateUserRequest struct {
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name"`
	Password    string        `json:"password"`
	Role        user.UserRole `json:"role"`
}

// UpdateUserRequest is the inbound patch payload of PATCH
// /api/users/{user_id}. omitempty means zero values are treated as
// "field not provided" by both the JSON decoder and the application layer:
// callers cannot clear DisplayName, Role, or Status through this DTO.
// Password is intentionally not exposed here; admin-initiated password
// resets require a dedicated endpoint.
type UpdateUserRequest struct {
	DisplayName string          `json:"display_name,omitempty"`
	Role        user.UserRole   `json:"role,omitempty"`
	Status      user.UserStatus `json:"status,omitempty"`
}

// UserResponse is the public-safe projection of a domain User used by every
// user-management response and /api/me. PasswordHash is excluded by
// construction (it is not even a field here) so accidental serialisation of
// the hash is impossible.
type UserResponse struct {
	ID          string          `json:"id"`
	Email       string          `json:"email"`
	DisplayName string          `json:"display_name"`
	Role        user.UserRole   `json:"role"`
	Status      user.UserStatus `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ListUsersResponse is the outbound payload of GET /api/users. The response
// is unpaginated by design; the user collection is expected to stay small
// enough that a full scan per admin list is acceptable.
type ListUsersResponse struct {
	Users []UserResponse `json:"users"`
}

// UserFromDomain projects a domain User onto UserResponse. The function
// silently drops PasswordHash; callers do not need to clear it before
// calling.
func UserFromDomain(u *user.User) UserResponse {
	return UserResponse{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        u.Role,
		Status:      u.Status,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}
