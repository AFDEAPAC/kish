package dto

import (
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/user"
)

// CreateUserRequest is the JSON body for POST /api/users.
type CreateUserRequest struct {
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name"`
	Password    string        `json:"password"`
	Role        user.UserRole `json:"role"`
}

// UpdateUserRequest is the JSON body for PATCH /api/users/{user_id}.
// Only non-zero fields are applied.
type UpdateUserRequest struct {
	DisplayName string        `json:"display_name,omitempty"`
	Role        user.UserRole `json:"role,omitempty"`
	Status      user.UserStatus `json:"status,omitempty"`
}

// UserResponse is the JSON representation of a user in API responses.
// PasswordHash is never included.
type UserResponse struct {
	ID          string          `json:"id"`
	Email       string          `json:"email"`
	DisplayName string          `json:"display_name"`
	Role        user.UserRole   `json:"role"`
	Status      user.UserStatus `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ListUsersResponse is returned by GET /api/users.
type ListUsersResponse struct {
	Users []UserResponse `json:"users"`
}

// UserFromDomain maps a domain User to a UserResponse DTO.
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
