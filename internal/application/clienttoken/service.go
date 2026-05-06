// Package clienttoken provides the application-layer service for client token lifecycle.
//
// ClientTokenService handles creation, listing, and revocation of client tokens.
// It enforces that raw tokens are never stored, that scopes are valid, and that
// users can only manage their own tokens.
package clienttoken

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/clienttoken"
	"github.com/AFDEAPAC/kish/internal/infrastructure/security"
)

// ErrUnauthorized is returned when a user attempts to manage a token they do not own.
var ErrUnauthorized = errors.New("cannot manage another user's client token")

// CreateInput carries the fields for creating a new client token.
type CreateInput struct {
	UserID    string
	Name      string
	Scopes    []clienttoken.Scope
	ExpiresAt *time.Time
	Unlimited bool
}

// CreateResult contains the stored token metadata and the raw token (shown once).
type CreateResult struct {
	Token    *clienttoken.ClientToken
	RawToken string
}

// Service provides client token management operations.
type Service struct {
	repo   clienttoken.Repository
	prefix string
}

// NewService constructs a ClientTokenService.
// prefix is the token prefix (e.g. "kish"), configured via ClientTokenConfig.
func NewService(repo clienttoken.Repository, prefix string) *Service {
	return &Service{repo: repo, prefix: prefix}
}

// CreateToken generates and persists a new client token.
// The raw token is returned exactly once in CreateResult.RawToken.
func (s *Service) CreateToken(ctx context.Context, in CreateInput) (*CreateResult, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !in.Unlimited && in.ExpiresAt == nil {
		return nil, fmt.Errorf("expires_at is required when unlimited is false")
	}
	for _, scope := range in.Scopes {
		if !clienttoken.ValidScope(scope) {
			return nil, fmt.Errorf("invalid scope %q: must be testcase:read or testcase:write", scope)
		}
	}
	if len(in.Scopes) == 0 {
		return nil, fmt.Errorf("at least one scope is required")
	}

	rawToken, err := security.GenerateOpaqueToken(s.prefix)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	t := &clienttoken.ClientToken{
		UserID:      in.UserID,
		Name:        in.Name,
		TokenPrefix: security.TokenPrefix(rawToken),
		TokenHash:   security.HashToken(rawToken),
		Scopes:      in.Scopes,
		ExpiresAt:   in.ExpiresAt,
		Unlimited:   in.Unlimited,
	}

	created, err := s.repo.Create(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("persist token: %w", err)
	}

	return &CreateResult{Token: created, RawToken: rawToken}, nil
}

// ListTokens returns all client tokens owned by the given user.
func (s *Service) ListTokens(ctx context.Context, userID string) ([]*clienttoken.ClientToken, error) {
	tokens, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	return tokens, nil
}

// RevokeToken revokes the token identified by tokenID.
// Returns ErrUnauthorized when the token does not belong to requestingUserID.
func (s *Service) RevokeToken(ctx context.Context, tokenID, requestingUserID string) error {
	t, err := s.repo.FindByID(ctx, tokenID)
	if err != nil {
		return err
	}
	if t.UserID != requestingUserID {
		return ErrUnauthorized
	}
	return s.repo.Revoke(ctx, tokenID)
}

// LookupByRawToken hashes the raw token and looks it up in the repository.
// Returns the token if found and valid; returns an error otherwise.
// Used by the auth middleware to resolve client token principals.
func (s *Service) LookupByRawToken(ctx context.Context, rawToken string) (*clienttoken.ClientToken, error) {
	hash := security.HashToken(rawToken)
	t, err := s.repo.FindByTokenHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if !t.IsValid(time.Now().UTC()) {
		return nil, clienttoken.ErrNotFound
	}
	return t, nil
}
