// Package bootstrap provides the initial admin creation service.
//
// BootstrapService runs once at API server startup. If no admin user exists
// and bootstrap is enabled, it creates the initial admin account from config.
// Subsequent startups are no-ops when an admin already exists.
package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/AFDEAPAC/kish/internal/domain/user"
)

// Options contains the startup data required to create the initial admin.
//
// The outer config loader owns file and environment parsing; the bootstrap use
// case only receives the values it needs for the application decision.
type Options struct {
	Enabled          bool
	AdminEmail       string
	AdminPassword    string
	AdminDisplayName string
}

// PasswordHasher hashes the bootstrap admin password without exposing the
// concrete security implementation to the use case.
type PasswordHasher interface {
	Hash(plaintext string) (string, error)
}

// Logger records bootstrap outcomes for the outer startup workflow.
type Logger interface {
	Printf(format string, v ...any)
}

type noopLogger struct{}

func (noopLogger) Printf(string, ...any) {}

// Service performs at-most-once initial admin creation.
//
// Service is invoked from main during API startup, before the HTTP server
// begins accepting requests, and is intentionally fire-and-forget: any
// failure to create the admin is logged but never returned, so a
// misconfigured bootstrap section cannot block the rest of the API from
// starting up. Two instances racing on first start are reconciled via
// user.ErrEmailConflict (see createAdmin).
type Service struct {
	userRepo  user.Repository
	hasher    PasswordHasher
	options   Options
	logger    Logger
	minPwdLen int
}

// NewService wires Service with its ports and configuration. loggers is
// variadic so the call site can omit logging in tests; only the first
// non-nil entry is used and a no-op logger is the default.
func NewService(userRepo user.Repository, hasher PasswordHasher, options Options, minPwdLen int, loggers ...Logger) *Service {
	logger := Logger(noopLogger{})
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &Service{
		userRepo:  userRepo,
		hasher:    hasher,
		options:   options,
		logger:    logger,
		minPwdLen: minPwdLen,
	}
}

// Run checks whether an admin exists and creates the bootstrap admin if needed.
// It logs the outcome but does not return an error that would prevent server startup.
func (s *Service) Run(ctx context.Context) {
	count, err := s.userRepo.CountByRole(ctx, user.RoleAdmin)
	if err != nil {
		s.logger.Printf("[bootstrap] error checking admin count: %v — skipping bootstrap", err)
		return
	}

	if count > 0 {
		s.logger.Printf("[bootstrap] %d admin(s) already exist — skipping bootstrap", count)
		return
	}

	if !s.options.Enabled {
		s.logger.Printf("[bootstrap] no admin users exist and bootstrap is disabled — admin-only operations will be unavailable")
		return
	}

	if err := s.createAdmin(ctx); err != nil {
		s.logger.Printf("[bootstrap] failed to create initial admin: %v", err)
		return
	}
	s.logger.Printf("[bootstrap] initial admin created: %s", s.options.AdminEmail)
}

func (s *Service) createAdmin(ctx context.Context) error {
	if s.options.AdminEmail == "" {
		return fmt.Errorf("bootstrap.admin_email is required")
	}
	if s.options.AdminPassword == "" {
		return fmt.Errorf("bootstrap.admin_password is required")
	}
	if len(s.options.AdminPassword) < s.minPwdLen {
		return fmt.Errorf("bootstrap password must be at least %d characters", s.minPwdLen)
	}

	hash, err := s.hasher.Hash(s.options.AdminPassword)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}

	displayName := s.options.AdminDisplayName
	if displayName == "" {
		displayName = "Kish Admin"
	}

	u := &user.User{
		Email:        s.options.AdminEmail,
		DisplayName:  displayName,
		Role:         user.RoleAdmin,
		Status:       user.StatusActive,
		PasswordHash: hash,
	}

	_, err = s.userRepo.Create(ctx, u)
	if err != nil {
		if errors.Is(err, user.ErrEmailConflict) {
			// Another instance created the admin concurrently; treat as success.
			s.logger.Printf("[bootstrap] admin email already exists — skipping duplicate creation")
			return nil
		}
		return fmt.Errorf("persist bootstrap admin: %w", err)
	}
	return nil
}
