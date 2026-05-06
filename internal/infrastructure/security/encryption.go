package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// TokenCipher encrypts raw client tokens for owner reveal while keeping token
// lookup based on the existing one-way hash.
type TokenCipher struct {
	gcm cipher.AEAD
}

// NewTokenCipher constructs an AES-GCM cipher from a configured secret. The
// secret is hashed to a 32-byte AES key so operators can provide normal strings.
func NewTokenCipher(secret string) (*TokenCipher, error) {
	if secret == "" {
		return nil, nil
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("token cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("token cipher gcm: %w", err)
	}
	return &TokenCipher{gcm: gcm}, nil
}

// Encrypt returns base64(nonce || ciphertext).
func (c *TokenCipher) Encrypt(plaintext string) (string, error) {
	if c == nil {
		return "", nil
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("token encrypt nonce: %w", err)
	}
	out := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt.
func (c *TokenCipher) Decrypt(encoded string) (string, error) {
	if c == nil || encoded == "" {
		return "", fmt.Errorf("token content is unavailable")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("token decrypt decode: %w", err)
	}
	nonceSize := c.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("token decrypt: ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := c.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("token decrypt: %w", err)
	}
	return string(plaintext), nil
}
