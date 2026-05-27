// Package clienttoken defines the ClientToken domain entity and Scope types.
//
// Client tokens (also called API tokens) are long-lived credentials used by
// kish upload and machine-to-machine integrations. They are not suitable for
// interactive web dashboard sessions, which use JWT.
//
// Token storage rule: the SHA-256 hash (TokenHash) is the lookup key and is
// always persisted; the raw token value is never stored. When the deployment
// configures a token encryption key, the application layer additionally
// persists the raw token under EncryptedToken so the owner can reveal it
// later. Without that key tokens are hash-only and not recoverable.
// EncryptedToken is a secret in plaintext form once decrypted and must never
// be logged or returned by list/lookup endpoints.
package clienttoken

import (
	"errors"
	"time"
)

// Scope restricts the operations a client token may perform.
type Scope string

const (
	// ScopeTestCaseRead allows reading TestCases and artifact metadata.
	ScopeTestCaseRead Scope = "testcase:read"

	// ScopeTestCaseWrite allows creating TestCases and uploading artifacts.
	// Implies read access.
	ScopeTestCaseWrite Scope = "testcase:write"
)

// ErrInvalidScope is returned when an unrecognised scope value is encountered.
var ErrInvalidScope = errors.New("invalid scope")

// ValidScope reports whether s is a supported scope value.
func ValidScope(s Scope) bool {
	return s == ScopeTestCaseRead || s == ScopeTestCaseWrite
}

// ClientToken represents a stored client token credential.
//
// TokenHash is the SHA-256 of the raw token and is the only lookup key.
// TokenPrefix holds the first 8 characters of the raw token for UI display
// (it is not secret but must not be used for authentication). EncryptedToken
// is empty when the deployment runs without a token encryption key; when set
// it must be treated as a high-value secret and decrypted only for the owner
// during reveal. Both TokenHash and EncryptedToken are excluded from JSON
// responses; serializers in interfaces/http/dto enforce that boundary.
type ClientToken struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Name        string `json:"name"`
	TokenPrefix string `json:"token_prefix"`
	// TokenHash is the SHA-256 of the raw token. Never log or return in API.
	TokenHash string `json:"-"`
	// EncryptedToken stores the raw token under the configured token
	// encryption key for owner-reveal flows. Empty when encryption is
	// disabled. Must be decrypted only by the owning user.
	EncryptedToken string     `json:"-"`
	Scopes         []Scope    `json:"scopes"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Unlimited      bool       `json:"unlimited"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// IsValid reports whether the token may be used for authentication.
// A token is invalid when it has been revoked or has passed its expiration.
func (t *ClientToken) IsValid(now time.Time) bool {
	if t.RevokedAt != nil {
		return false
	}
	if !t.Unlimited && t.ExpiresAt != nil && now.After(*t.ExpiresAt) {
		return false
	}
	return true
}

// HasScope reports whether the token grants the given scope.
// ScopeTestCaseWrite also satisfies ScopeTestCaseRead.
func (t *ClientToken) HasScope(required Scope) bool {
	for _, s := range t.Scopes {
		if s == required {
			return true
		}
		// write scope implies read access.
		if required == ScopeTestCaseRead && s == ScopeTestCaseWrite {
			return true
		}
	}
	return false
}

// CreateInput carries the fields required to create a new client token.
type CreateInput struct {
	UserID    string
	Name      string
	Scopes    []Scope
	ExpiresAt *time.Time
	Unlimited bool
}
