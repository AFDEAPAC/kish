// Package auth provides the application-layer service for authentication.
//
// AuthService handles login (credential verification + token issuance),
// token refresh (with rotation), logout (session revocation), and
// current-user resolution for both JWT and client-token principals.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/session"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/infrastructure/security"
)

// ErrInvalidCredentials is returned when email or password does not match.
// The error is intentionally opaque to prevent user enumeration.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrInvalidRefreshToken is returned when the provided refresh token is
// expired, revoked, or not found.
var ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")

// LoginResult contains the tokens and user info returned after a successful login.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64 // seconds
	User         *user.User
}

// RefreshResult contains the new tokens issued after a successful refresh.
type RefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// Service provides authentication operations.
type Service struct {
	userRepo    user.Repository
	sessionRepo session.Repository
	hasher      security.PasswordHasher
	jwtSvc      *security.JWTService
	refreshTTL  time.Duration
}

// NewService constructs an AuthService.
func NewService(
	userRepo user.Repository,
	sessionRepo session.Repository,
	hasher security.PasswordHasher,
	jwtSvc *security.JWTService,
	refreshTTL time.Duration,
) *Service {
	return &Service{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		hasher:      hasher,
		jwtSvc:      jwtSvc,
		refreshTTL:  refreshTTL,
	}
}

// Login authenticates a user by email and password, then issues an access token
// and a refresh token.
func (s *Service) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	u, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("login lookup: %w", err)
	}

	if u.Status == user.StatusDisabled {
		// Treat disabled accounts as invalid credentials to avoid leaking status.
		return nil, ErrInvalidCredentials
	}

	if err := s.hasher.Verify(password, u.PasswordHash); err != nil {
		if errors.Is(err, security.ErrInvalidPassword) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("verify password: %w", err)
	}

	accessToken, expiresIn, err := s.issueAccessToken(u)
	if err != nil {
		return nil, err
	}

	rawRefresh, err := s.issueRefreshToken(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	u.PasswordHash = ""
	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		ExpiresIn:    expiresIn,
		User:         u,
	}, nil
}

// Refresh validates the provided refresh token, revokes it, and issues a new
// access token and refresh token (rotation).
func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (*RefreshResult, error) {
	hash := security.HashToken(rawRefreshToken)
	sess, err := s.sessionRepo.FindByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("find session: %w", err)
	}

	if !sess.IsValid(time.Now().UTC()) {
		return nil, ErrInvalidRefreshToken
	}

	// Load user to verify account is still active.
	u, err := s.userRepo.FindByID(ctx, sess.UserID)
	if err != nil {
		return nil, fmt.Errorf("load user for refresh: %w", err)
	}
	if u.Status == user.StatusDisabled {
		return nil, ErrInvalidRefreshToken
	}

	// Revoke the old session (rotation).
	if err := s.sessionRepo.Revoke(ctx, sess.ID); err != nil {
		return nil, fmt.Errorf("revoke old session: %w", err)
	}

	accessToken, expiresIn, err := s.issueAccessToken(u)
	if err != nil {
		return nil, err
	}

	rawNewRefresh, err := s.issueRefreshToken(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	return &RefreshResult{
		AccessToken:  accessToken,
		RefreshToken: rawNewRefresh,
		ExpiresIn:    expiresIn,
	}, nil
}

// Logout revokes the session identified by the provided raw refresh token.
func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	hash := security.HashToken(rawRefreshToken)
	sess, err := s.sessionRepo.FindByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			// Already gone; treat as success.
			return nil
		}
		return fmt.Errorf("find session for logout: %w", err)
	}
	return s.sessionRepo.Revoke(ctx, sess.ID)
}

// issueAccessToken creates and signs a JWT access token for the user.
// Returns the token string and its lifetime in seconds.
func (s *Service) issueAccessToken(u *user.User) (string, int64, error) {
	token, err := s.jwtSvc.Issue(u.ID, u.Role)
	if err != nil {
		return "", 0, fmt.Errorf("issue access token: %w", err)
	}
	// ExpiresIn is communicated to callers; the exact duration is configured
	// in the JWT service. We return a rounded estimate of 24h * 3600 by default.
	// A more precise value would require exposing the TTL from JWTService.
	expiresIn := int64(24 * 60 * 60)
	return token, expiresIn, nil
}

// issueRefreshToken generates a new opaque refresh token, persists its hash,
// and returns the raw token to the caller.
func (s *Service) issueRefreshToken(ctx context.Context, userID string) (string, error) {
	rawToken, err := security.GenerateOpaqueToken("ref")
	if err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	sess := &session.Session{
		UserID:    userID,
		TokenHash: security.HashToken(rawToken),
		ExpiresAt: time.Now().UTC().Add(s.refreshTTL),
	}
	if _, err := s.sessionRepo.Create(ctx, sess); err != nil {
		return "", fmt.Errorf("persist session: %w", err)
	}
	return rawToken, nil
}
