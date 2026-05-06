// Package user provides the application-layer service for user management.
//
// UserService coordinates user creation, retrieval, update, and soft-disable
// operations. It enforces the last-admin invariant and delegates persistence
// to the domain repository interface.
package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/infrastructure/security"
)

// ErrLastAdmin is returned when an operation would leave the system with no admin.
var ErrLastAdmin = errors.New("cannot remove or demote the last admin user")

// ErrIncorrectPassword is returned when the provided current password does not match.
var ErrIncorrectPassword = errors.New("current password is incorrect")

// CreateInput carries the caller-supplied data for creating a new user.
type CreateInput struct {
	Email       string
	DisplayName string
	Password    string
	Role        user.UserRole
}

// Service provides user management operations.
type Service struct {
	repo   user.Repository
	hasher security.PasswordHasher
	minPwdLen int
}

// NewService constructs a UserService.
func NewService(repo user.Repository, hasher security.PasswordHasher, minPwdLen int) *Service {
	return &Service{repo: repo, hasher: hasher, minPwdLen: minPwdLen}
}

// CreateUser validates input, hashes the password, and persists a new user.
func (s *Service) CreateUser(ctx context.Context, in CreateInput) (*user.User, error) {
	if in.Email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if !in.Role.IsValid() {
		return nil, fmt.Errorf("invalid role: must be admin or developer")
	}
	if len(in.Password) < s.minPwdLen {
		return nil, fmt.Errorf("password must be at least %d characters", s.minPwdLen)
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &user.User{
		Email:        in.Email,
		DisplayName:  in.DisplayName,
		Role:         in.Role,
		Status:       user.StatusActive,
		PasswordHash: hash,
	}
	created, err := s.repo.Create(ctx, u)
	if err != nil {
		if errors.Is(err, user.ErrEmailConflict) {
			return nil, user.ErrEmailConflict
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	// Clear hash before returning to caller.
	created.PasswordHash = ""
	return created, nil
}

// GetUser returns a user by ID with the password hash cleared.
func (s *Service) GetUser(ctx context.Context, id string) (*user.User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = ""
	return u, nil
}

// ListUsers returns all users with password hashes cleared.
func (s *Service) ListUsers(ctx context.Context) ([]*user.User, error) {
	users, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	for _, u := range users {
		u.PasswordHash = ""
	}
	return users, nil
}

// UpdateUser applies the non-zero fields of input to the user identified by id.
// Prevents demoting the last admin.
func (s *Service) UpdateUser(ctx context.Context, id string, input user.UpdateInput) (*user.User, error) {
	if err := s.guardLastAdmin(ctx, id, input); err != nil {
		return nil, err
	}
	updated, err := s.repo.Update(ctx, id, input)
	if err != nil {
		return nil, err
	}
	updated.PasswordHash = ""
	return updated, nil
}

// DisableUser sets the user's status to disabled and revoking all sessions is
// handled by the auth service when needed.
// Prevents disabling the last admin.
func (s *Service) DisableUser(ctx context.Context, id string) (*user.User, error) {
	return s.UpdateUser(ctx, id, user.UpdateInput{Status: user.StatusDisabled})
}

// ChangePassword verifies the user's current password and sets a new one.
func (s *Service) ChangePassword(ctx context.Context, userID, currentPwd, newPwd string) error {
	if len(newPwd) < s.minPwdLen {
		return fmt.Errorf("password must be at least %d characters", s.minPwdLen)
	}

	// Load the user with the password hash for verification.
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := s.hasher.Verify(currentPwd, u.PasswordHash); err != nil {
		if errors.Is(err, security.ErrInvalidPassword) {
			return ErrIncorrectPassword
		}
		return fmt.Errorf("verify password: %w", err)
	}

	newHash, err := s.hasher.Hash(newPwd)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	return s.repo.UpdatePasswordHash(ctx, userID, newHash)
}

// guardLastAdmin returns ErrLastAdmin when the pending update would leave
// the system with zero active admin users.
func (s *Service) guardLastAdmin(ctx context.Context, id string, input user.UpdateInput) error {
	// Only relevant when the update demotes the role or disables the account.
	willDemote := input.Role != "" && input.Role != user.RoleAdmin
	willDisable := input.Status == user.StatusDisabled

	if !willDemote && !willDisable {
		return nil
	}

	// Check whether this user is currently an admin.
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.Role != user.RoleAdmin {
		return nil
	}

	// Count remaining admins.
	count, err := s.repo.CountByRole(ctx, user.RoleAdmin)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if count <= 1 {
		return ErrLastAdmin
	}
	return nil
}
