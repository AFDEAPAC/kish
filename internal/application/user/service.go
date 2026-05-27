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

// PasswordHasher is the user-management use case's password port.
//
// Implementations hash and verify plaintext passwords without leaking the
// concrete algorithm or mismatch sentinel into application logic.
type PasswordHasher interface {
	Hash(plaintext string) (string, error)
	VerifyPassword(plaintext, hash string) (bool, error)
}

// Service orchestrates user CRUD, password change, and account disable.
//
// Service owns the last-admin invariant (guardLastAdmin) and the rule that
// PasswordHash never leaves the service boundary in plain form: every
// returned user has PasswordHash cleared. Authorization (admin-only,
// JWT-only) is enforced upstream by the HTTP middleware and routes;
// callers reaching the service are trusted to have already been
// authenticated and authorised.
type Service struct {
	repo      user.Repository
	hasher    PasswordHasher
	minPwdLen int
}

// NewService wires Service. minPwdLen is the deployment-configured minimum
// password length applied to CreateUser and ChangePassword; values below
// the configured floor are rejected at validation time.
func NewService(repo user.Repository, hasher PasswordHasher, minPwdLen int) *Service {
	return &Service{repo: repo, hasher: hasher, minPwdLen: minPwdLen}
}

// CreateUser validates input, hashes the password, and persists a new
// active user.
//
// CreateUser returns user.ErrEmailConflict when the email is already taken.
// Validation errors (empty email, invalid role, short password) are wrapped
// fmt.Errorf values intended for direct exposure to admin callers. Other
// errors represent infrastructure failures.
//
// The returned user has PasswordHash cleared. Callers must not log
// in.Password and must trust the configured PasswordHasher to keep the
// concrete algorithm opaque.
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

// DisableUser flips the user's status to disabled. Returns ErrLastAdmin if
// disabling would leave the system with zero admins.
//
// DisableUser does NOT revoke the user's existing refresh sessions. Any
// previously issued refresh token continues to verify against the session
// store until it expires; auth.Service.Refresh re-checks user status on
// every rotation and rejects disabled accounts at that point. JWT access
// tokens already in flight remain valid until their exp claim because the
// project does not maintain a JWT revocation list. Callers that need an
// immediate, hard sign-out must additionally:
//   - revoke or delete the user's session records, and
//   - rely on access-token TTL being short enough to absorb the gap.
//
// Keeping session revocation out of this method preserves a clean
// Clean-Architecture boundary: user.Service does not import session
// infrastructure. The HTTP admin handler (the only DisableUser caller) is
// responsible for the cleanup step.
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

	ok, err := s.hasher.VerifyPassword(currentPwd, u.PasswordHash)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return ErrIncorrectPassword
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

	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.Role != user.RoleAdmin {
		return nil
	}

	count, err := s.repo.CountByRole(ctx, user.RoleAdmin)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if count <= 1 {
		return ErrLastAdmin
	}
	return nil
}
