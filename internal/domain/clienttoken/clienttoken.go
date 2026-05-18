// Package clienttoken defines the ClientToken domain entity and Scope types.
//
// Client tokens (also called API tokens) are long-lived credentials used by
// kish upload and machine-to-machine integrations. They are not suitable for
// interactive web dashboard sessions, which use JWT.
//
// Token storage rule: the SHA-256 hash is persisted for lookup, and newer
// tokens may also persist an encrypted raw token for owner reveal.
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
// The raw token is never stored. Only TokenHash (SHA-256) and TokenPrefix
// (first 8 characters of the raw token, for display) are persisted.
type ClientToken struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Name           string     `json:"name"`
	TokenPrefix    string     `json:"token_prefix"`
	TokenHash      string     `json:"-"`
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
