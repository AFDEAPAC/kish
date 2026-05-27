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

// TokenCipher encrypts raw client tokens for the owner-reveal flow while
// authentication continues to rely on the one-way hash.
//
// A nil *TokenCipher is a valid sentinel meaning "encryption disabled".
// Methods on a nil receiver are still safe to call: Encrypt returns an
// empty ciphertext and Decrypt returns an unavailable-token error. This
// keeps callers free of `if cipher != nil` branches.
type TokenCipher struct {
	gcm cipher.AEAD
}

// NewTokenCipher constructs an AES-256-GCM cipher from a deployment secret.
//
// secret is loaded from auth.client_token_encryption_key in config. An empty
// secret deliberately returns (nil, nil) so callers can opt out of
// encryption (hash-only mode); see TokenCipher's doc for the nil-receiver
// contract. The secret is normalised through SHA-256 to a 32-byte AES key so
// operators can supply arbitrary-length passphrases without worrying about
// key length.
//
// Rotating the secret invalidates every existing EncryptedToken record:
// reveal stops working for tokens encrypted under the old key, but
// authentication via TokenHash continues to succeed.
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

// Encrypt seals plaintext under AES-256-GCM with a fresh random nonce and
// returns base64(nonce || ciphertext). On a nil receiver Encrypt returns
// ("", nil) so callers can write EncryptedToken unconditionally when the
// deployment runs without an encryption key.
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

// Decrypt reverses Encrypt and authenticates the ciphertext via GCM.
//
// Decrypt returns an "unavailable" error in two distinct situations that
// callers must collapse into the same user-visible response:
//   - the cipher is disabled (nil receiver) or the record was stored
//     without ciphertext (empty input);
//   - the payload is malformed, truncated, or has been tampered with.
//
// Encoding the difference would let an attacker probe encryption state.
// Higher layers translate any error from Decrypt into
// application/clienttoken.ErrTokenContentUnavailable.
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
