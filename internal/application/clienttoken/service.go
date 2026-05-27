// Package clienttoken provides the application-layer service for client
// token lifecycle: create, list, reveal, revoke, and authenticated lookup.
//
// Token storage rules enforced here:
//   - Only the SHA-256 hash (TokenHash) is required for lookup.
//   - The raw token is returned to the owner exactly once at creation.
//   - When a token encryption key is configured the raw token is also
//     persisted encrypted under that key so the owner can reveal it later;
//     without that key the token is hash-only and reveal returns
//     ErrTokenContentUnavailable.
//
// Authorization rules enforced here:
//   - Reveal and Revoke require the requesting user to own the token.
//   - CreateInput.UserID must already be a trusted, authenticated user id
//     supplied by the interface layer; Service does not authenticate.
//   - LookupByRawToken is used by the HTTP auth middleware to resolve a
//     bearer credential into a principal and must never return a revoked or
//     expired token.
package clienttoken

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/clienttoken"
)

// ErrUnauthorized is returned when a user attempts to manage a token they do not own.
var ErrUnauthorized = errors.New("cannot manage another user's client token")

// ErrTokenContentUnavailable is returned when a token cannot be revealed because
// it is legacy, revoked, expired, or the encryption key is unavailable.
var ErrTokenContentUnavailable = errors.New("token content is unavailable; create a new token")

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

// RevealResult contains stored token metadata and the decrypted raw token.
type RevealResult struct {
	Token    *clienttoken.ClientToken
	RawToken string
}

// TokenDetail contains client token metadata plus the retrievable raw token
// when encrypted token storage is available for that record.
type TokenDetail struct {
	Token    *clienttoken.ClientToken
	RawToken *string
}

// Service orchestrates client-token lifecycle and bearer-token lookup.
//
// Service is the only component that ever sees raw client tokens between
// generation and the owner's reveal flow. It uses two cryptographic ports:
// tokenCodec for one-way hashing and display-prefix derivation, and an
// optional tokenCipher for owner-reveal encryption. When cipher is nil the
// service runs in hash-only mode and reveal is unavailable.
type Service struct {
	repo   clienttoken.Repository
	prefix string
	codec  tokenCodec
	cipher tokenCipher
}

// tokenCodec generates, hashes, and derives display prefixes for raw client
// tokens without exposing the concrete cryptographic implementation.
type tokenCodec interface {
	Generate(prefix string) (string, error)
	Hash(raw string) string
	Prefix(raw string) string
}

// tokenCipher provides symmetric encryption of the raw token for the
// owner-reveal flow. Implementations must treat ciphertext as a high-value
// secret and must not leak plaintext via error messages.
type tokenCipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(encoded string) (string, error)
}

// NewService wires Service with its persistence repository, the deployment
// token prefix (e.g. "kish"), the cryptographic codec, and an optional
// cipher. Passing zero cipher arguments runs Service in hash-only mode; in
// that mode RevealToken returns ErrTokenContentUnavailable for every token.
// Passing more than one cipher is undefined and only the first is used.
func NewService(repo clienttoken.Repository, prefix string, codec tokenCodec, ciphers ...tokenCipher) *Service {
	var cipher tokenCipher
	if len(ciphers) > 0 {
		cipher = ciphers[0]
	}
	return &Service{repo: repo, prefix: prefix, codec: codec, cipher: cipher}
}

// CreateToken generates a new raw client token, hashes it for lookup,
// optionally encrypts it for owner reveal, and persists the token record.
//
// CreateResult.RawToken is the only place the raw token is ever returned;
// callers must surface it to the owner exactly once and never log it. After
// the response is sent the raw value can only be recovered through
// RevealToken, and only when the deployment has a token encryption key.
//
// CreateInput.UserID is trusted as-is. The caller must have authenticated
// the user via JWT (client tokens are not allowed to create more tokens;
// that policy is enforced in the HTTP route layer).
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

	rawToken, err := s.codec.Generate(s.prefix)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	encryptedToken := ""
	if s.cipher != nil {
		encryptedToken, err = s.cipher.Encrypt(rawToken)
		if err != nil {
			return nil, fmt.Errorf("encrypt token: %w", err)
		}
	}

	t := &clienttoken.ClientToken{
		UserID:         in.UserID,
		Name:           in.Name,
		TokenPrefix:    s.codec.Prefix(rawToken),
		TokenHash:      s.codec.Hash(rawToken),
		EncryptedToken: encryptedToken,
		Scopes:         in.Scopes,
		ExpiresAt:      in.ExpiresAt,
		Unlimited:      in.Unlimited,
	}

	created, err := s.repo.Create(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("persist token: %w", err)
	}

	return &CreateResult{Token: created, RawToken: rawToken}, nil
}

func (s *Service) rawTokenFor(t *clienttoken.ClientToken) *string {
	if t.RevokedAt != nil || t.EncryptedToken == "" || s.cipher == nil {
		return nil
	}
	raw, err := s.cipher.Decrypt(t.EncryptedToken)
	if err != nil {
		return nil
	}
	return &raw
}

// RevealToken returns the decrypted raw token to its owner.
//
// RevealToken returns:
//   - clienttoken.ErrNotFound when no token has the given id.
//   - ErrUnauthorized when the requesting user does not own the token.
//   - ErrTokenContentUnavailable when the token is revoked, expired, was
//     created in hash-only mode, the encryption key is unavailable, or the
//     ciphertext is corrupt. The four cases are collapsed so the caller
//     cannot distinguish "we don't have it" from "you can't have it".
func (s *Service) RevealToken(ctx context.Context, tokenID, requestingUserID string) (*RevealResult, error) {
	t, err := s.repo.FindByID(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	if t.UserID != requestingUserID {
		return nil, ErrUnauthorized
	}
	if !t.IsValid(time.Now().UTC()) || t.EncryptedToken == "" || s.cipher == nil {
		return nil, ErrTokenContentUnavailable
	}
	raw, err := s.cipher.Decrypt(t.EncryptedToken)
	if err != nil {
		return nil, ErrTokenContentUnavailable
	}
	return &RevealResult{Token: t, RawToken: raw}, nil
}

// ListTokens returns all client tokens owned by the given user, including a raw
// token value when the record has encrypted token storage available.
func (s *Service) ListTokens(ctx context.Context, userID string) ([]TokenDetail, error) {
	tokens, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	out := make([]TokenDetail, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, TokenDetail{Token: t, RawToken: s.rawTokenFor(t)})
	}
	return out, nil
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
	hash := s.codec.Hash(rawToken)
	t, err := s.repo.FindByTokenHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if !t.IsValid(time.Now().UTC()) {
		return nil, clienttoken.ErrNotFound
	}
	now := time.Now().UTC()
	// Last-used tracking is best-effort observability; a valid client token
	// must not fail authentication because usage metadata could not be updated.
	if err := s.repo.TouchLastUsed(ctx, t.ID, now); err == nil {
		t.LastUsedAt = &now
		t.UpdatedAt = now
	}
	return t, nil
}
