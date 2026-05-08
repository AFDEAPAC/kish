package dto

import (
	"time"

	appClientToken "github.com/AFDEAPAC/kish/internal/application/clienttoken"
	"github.com/AFDEAPAC/kish/internal/domain/clienttoken"
)

// CreateClientTokenRequest is the JSON body for POST /api/me/client-tokens.
type CreateClientTokenRequest struct {
	Name      string              `json:"name"`
	ExpiresAt *time.Time          `json:"expires_at,omitempty"`
	Unlimited bool                `json:"unlimited"`
	Scopes    []clienttoken.Scope `json:"scopes"`
}

// CreateClientTokenResponse is returned on successful POST /api/me/client-tokens.
// The raw Token field is shown exactly once and never returned again.
type CreateClientTokenResponse struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Token       string              `json:"token"`
	TokenPrefix string              `json:"token_prefix"`
	Scopes      []clienttoken.Scope `json:"scopes"`
	ExpiresAt   *time.Time          `json:"expires_at,omitempty"`
	Unlimited   bool                `json:"unlimited"`
	CreatedAt   time.Time           `json:"created_at"`
}

// ClientTokenMetaResponse is the list representation of a client token.
// Token is null for legacy records that cannot be decrypted from encrypted
// token storage.
type ClientTokenMetaResponse struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Token       *string             `json:"token"`
	TokenPrefix string              `json:"token_prefix"`
	Scopes      []clienttoken.Scope `json:"scopes"`
	ExpiresAt   *time.Time          `json:"expires_at,omitempty"`
	Unlimited   bool                `json:"unlimited"`
	RevokedAt   *time.Time          `json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time          `json:"last_used_at,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
}

// RevealClientTokenResponse includes the raw token for an owned token reveal.
type RevealClientTokenResponse struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Token       string              `json:"token"`
	TokenPrefix string              `json:"token_prefix"`
	Scopes      []clienttoken.Scope `json:"scopes"`
	ExpiresAt   *time.Time          `json:"expires_at,omitempty"`
	Unlimited   bool                `json:"unlimited"`
	RevokedAt   *time.Time          `json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time          `json:"last_used_at,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
}

// ListClientTokensResponse is returned by GET /api/me/client-tokens.
type ListClientTokensResponse struct {
	Tokens []ClientTokenMetaResponse `json:"tokens"`
}

// RevealClientTokenFromDomain maps a domain ClientToken and raw value to the reveal DTO.
func RevealClientTokenFromDomain(t *clienttoken.ClientToken, raw string) RevealClientTokenResponse {
	return RevealClientTokenResponse{
		ID:          t.ID,
		Name:        t.Name,
		Token:       raw,
		TokenPrefix: t.TokenPrefix,
		Scopes:      t.Scopes,
		ExpiresAt:   t.ExpiresAt,
		Unlimited:   t.Unlimited,
		RevokedAt:   t.RevokedAt,
		LastUsedAt:  t.LastUsedAt,
		CreatedAt:   t.CreatedAt,
	}
}

// ClientTokenMetaFromDetail maps an application token detail to the list DTO.
func ClientTokenMetaFromDetail(detail appClientToken.TokenDetail) ClientTokenMetaResponse {
	t := detail.Token
	return ClientTokenMetaResponse{
		ID:          t.ID,
		Name:        t.Name,
		Token:       detail.RawToken,
		TokenPrefix: t.TokenPrefix,
		Scopes:      t.Scopes,
		ExpiresAt:   t.ExpiresAt,
		Unlimited:   t.Unlimited,
		RevokedAt:   t.RevokedAt,
		LastUsedAt:  t.LastUsedAt,
		CreatedAt:   t.CreatedAt,
	}
}
