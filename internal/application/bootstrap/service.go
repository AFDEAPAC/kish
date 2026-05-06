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
	"log"

	"github.com/AFDEAPAC/kish/internal/config"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/infrastructure/security"
)

// Service handles the one-time bootstrap admin creation.
type Service struct {
	userRepo user.Repository
	hasher   security.PasswordHasher
	cfg      config.BootstrapConfig
	minPwdLen int
}

// NewService constructs a BootstrapService.
func NewService(userRepo user.Repository, hasher security.PasswordHasher, cfg config.BootstrapConfig, minPwdLen int) *Service {
	return &Service{
		userRepo:  userRepo,
		hasher:    hasher,
		cfg:       cfg,
		minPwdLen: minPwdLen,
	}
}

// Run checks whether an admin exists and creates the bootstrap admin if needed.
// It logs the outcome but does not return an error that would prevent server startup.
func (s *Service) Run(ctx context.Context) {
	count, err := s.userRepo.CountByRole(ctx, user.RoleAdmin)
	if err != nil {
		log.Printf("[bootstrap] error checking admin count: %v — skipping bootstrap", err)
		return
	}

	if count > 0 {
		log.Printf("[bootstrap] %d admin(s) already exist — skipping bootstrap", count)
		return
	}

	if !s.cfg.Enabled {
		log.Printf("[bootstrap] no admin users exist and bootstrap is disabled — admin-only operations will be unavailable")
		return
	}

	if err := s.createAdmin(ctx); err != nil {
		log.Printf("[bootstrap] failed to create initial admin: %v", err)
		return
	}
	log.Printf("[bootstrap] initial admin created: %s", s.cfg.AdminEmail)
}

func (s *Service) createAdmin(ctx context.Context) error {
	if s.cfg.AdminEmail == "" {
		return fmt.Errorf("bootstrap.admin_email is required")
	}
	if s.cfg.AdminPassword == "" {
		return fmt.Errorf("bootstrap.admin_password is required")
	}
	if len(s.cfg.AdminPassword) < s.minPwdLen {
		return fmt.Errorf("bootstrap password must be at least %d characters", s.minPwdLen)
	}

	hash, err := s.hasher.Hash(s.cfg.AdminPassword)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}

	displayName := s.cfg.AdminDisplayName
	if displayName == "" {
		displayName = "Kish Admin"
	}

	u := &user.User{
		Email:        s.cfg.AdminEmail,
		DisplayName:  displayName,
		Role:         user.RoleAdmin,
		Status:       user.StatusActive,
		PasswordHash: hash,
	}

	_, err = s.userRepo.Create(ctx, u)
	if err != nil {
		if errors.Is(err, user.ErrEmailConflict) {
			// Another instance created the admin concurrently; treat as success.
			log.Printf("[bootstrap] admin email already exists — skipping duplicate creation")
			return nil
		}
		return fmt.Errorf("persist bootstrap admin: %w", err)
	}
	return nil
}
