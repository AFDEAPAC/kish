package dto

import "github.com/AFDEAPAC/kish/internal/domain/user"

// LoginRequest is the JSON body for POST /api/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserInfo is the embedded user object in auth responses.
type UserInfo struct {
	ID          string        `json:"id"`
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name"`
	Role        user.UserRole `json:"role"`
}

// LoginResponse is returned on successful POST /api/auth/login.
type LoginResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	TokenType    string   `json:"token_type"`
	ExpiresIn    int64    `json:"expires_in"`
	User         UserInfo `json:"user"`
}

// RefreshRequest is the JSON body for POST /api/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshResponse is returned on successful POST /api/auth/refresh.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// LogoutRequest is the JSON body for POST /api/auth/logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// MeResponse is returned by GET /api/auth/me.
type MeResponse struct {
	ID          string        `json:"id"`
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name"`
	Role        user.UserRole `json:"role"`
	AuthMethod  string        `json:"auth_method"`
}
