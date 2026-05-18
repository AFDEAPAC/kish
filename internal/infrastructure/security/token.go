package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const opaqueTokenBytes = 16 // produces 32 hex characters

// OpaqueTokenService implements the application-layer opaque token port using
// cryptographically random token bytes and SHA-256 hashes.
type OpaqueTokenService struct{}

// NewOpaqueTokenService returns an opaque token service for refresh and client
// token workflows.
func NewOpaqueTokenService() OpaqueTokenService {
	return OpaqueTokenService{}
}

// Generate creates a URL-safe random token with the given prefix.
func (OpaqueTokenService) Generate(prefix string) (string, error) {
	return GenerateOpaqueToken(prefix)
}

// Hash returns the SHA-256 hash used for storage lookup.
func (OpaqueTokenService) Hash(raw string) string {
	return HashToken(raw)
}

// Prefix returns the display prefix for a raw token.
func (OpaqueTokenService) Prefix(raw string) string {
	return TokenPrefix(raw)
}

// GenerateOpaqueToken creates a URL-safe random token with the given prefix.
// Format: <prefix>_<32 hex characters>
// Example: kish_a3f8c2d1e5b6a7c8d9e0f1a2b3c4d5e6
//
// The raw token is intended to be shown to the user exactly once and never stored.
// Store only the SHA-256 hash returned by HashToken.
func GenerateOpaqueToken(prefix string) (string, error) {
	b := make([]byte, opaqueTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(b), nil
}

// HashToken returns the hex-encoded SHA-256 hash of the raw token.
// This hash is what gets stored in the database. The raw token must not be stored.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// TokenPrefix returns the first 8 characters of the raw token for display purposes.
// This gives users a recognizable prefix to identify their tokens without exposing
// the full value.
func TokenPrefix(raw string) string {
	if len(raw) < 8 {
		return raw
	}
	return raw[:8]
}
