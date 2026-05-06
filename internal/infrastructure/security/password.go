// Package security provides password hashing, JWT signing/verification, and
// opaque token generation utilities for the kish authentication system.
//
// All functions in this package operate on plaintext secrets only transiently;
// they must never be stored or logged in their raw form.
package security

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// ErrInvalidPassword is returned when a plaintext password does not match
// the stored bcrypt hash.
var ErrInvalidPassword = errors.New("invalid password")

// PasswordHasher defines the interface for password hashing and verification.
// Using an interface allows the application layer to depend on an abstraction
// rather than a specific algorithm, simplifying testing.
type PasswordHasher interface {
	// Hash returns a bcrypt hash of the plaintext password.
	Hash(plaintext string) (string, error)

	// Verify checks whether plaintext matches the given hash.
	// Returns ErrInvalidPassword when they do not match.
	Verify(plaintext, hash string) error
}

// BcryptHasher implements PasswordHasher using bcrypt.
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher returns a BcryptHasher with the default cost (12).
func NewBcryptHasher() *BcryptHasher {
	return &BcryptHasher{cost: bcryptCost}
}

// Hash generates a bcrypt hash of the plaintext password.
func (h *BcryptHasher) Hash(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), h.cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(b), nil
}

// Verify checks whether plaintext matches the stored bcrypt hash.
// Returns ErrInvalidPassword when they do not match.
func (h *BcryptHasher) Verify(plaintext, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidPassword
		}
		return fmt.Errorf("bcrypt verify: %w", err)
	}
	return nil
}
