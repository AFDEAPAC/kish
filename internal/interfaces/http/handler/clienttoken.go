package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	appClientToken "github.com/AFDEAPAC/kish/internal/application/clienttoken"
	"github.com/AFDEAPAC/kish/internal/domain/clienttoken"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// ClientTokenHandler adapts the client-token application service into the
// /api/me/client-tokens endpoints.
//
// Every route under /api/me/client-tokens is registered with RequireJWT in
// routes.go: client tokens are not allowed to create more client tokens,
// list other tokens, reveal them, or revoke them. The handler trusts the
// Principal.UserID populated by the auth middleware to scope all operations
// to the calling user.
//
// HTTP error mapping:
//   - appClientToken.ErrUnauthorized          -> 403
//   - clienttoken.ErrNotFound                  -> 404
//   - appClientToken.ErrTokenContentUnavailable -> 409
//   - validation failures from CreateToken     -> 400
//   - anything else                            -> 500
type ClientTokenHandler struct {
	svc *appClientToken.Service
}

// NewClientTokenHandler wires the handler. svc may be nil in tests that do
// not exercise client-token routes; nil dereferences will surface as 500.
func NewClientTokenHandler(svc *appClientToken.Service) *ClientTokenHandler {
	return &ClientTokenHandler{svc: svc}
}

// Create issues a new client token for the calling user.
//
// The raw token is returned exactly once inside CreateClientTokenResponse;
// once the response is sent the value cannot be recovered unless the
// deployment runs with a token encryption key (see RevealToken). 201 on
// success, 400 on validation failures from CreateToken, 500 otherwise.
func (h *ClientTokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())

	var req dto.CreateClientTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	result, err := h.svc.CreateToken(r.Context(), appClientToken.CreateInput{
		UserID:    p.UserID,
		Name:      req.Name,
		Scopes:    req.Scopes,
		ExpiresAt: req.ExpiresAt,
		Unlimited: req.Unlimited,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, dto.CreateClientTokenResponse{
		ID:          result.Token.ID,
		Name:        result.Token.Name,
		Token:       result.RawToken,
		TokenPrefix: result.Token.TokenPrefix,
		Scopes:      result.Token.Scopes,
		ExpiresAt:   result.Token.ExpiresAt,
		Unlimited:   result.Token.Unlimited,
		CreatedAt:   result.Token.CreatedAt,
	})
}

// Reveal returns the decrypted raw token to its owner.
//
// 200 with the raw token when reveal succeeds; 400 when token_id is missing;
// 403 when the calling user is not the owner; 404 when the token does not
// exist; 409 when the token cannot be revealed (revoked, expired, hash-only
// deployment, or ciphertext corrupt). The four 409 sub-cases are
// deliberately indistinguishable to avoid leaking storage state.
func (h *ClientTokenHandler) Reveal(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	tokenID := r.PathValue("token_id")
	if tokenID == "" {
		writeError(w, http.StatusBadRequest, "token_id is required")
		return
	}

	result, err := h.svc.RevealToken(r.Context(), tokenID, p.UserID)
	if err != nil {
		switch {
		case errors.Is(err, appClientToken.ErrUnauthorized):
			writeError(w, http.StatusForbidden, "cannot reveal another user's token")
		case errors.Is(err, clienttoken.ErrNotFound):
			writeError(w, http.StatusNotFound, "token not found")
		case errors.Is(err, appClientToken.ErrTokenContentUnavailable):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to reveal token")
		}
		return
	}

	writeJSON(w, http.StatusOK, dto.RevealClientTokenFromDomain(result.Token, result.RawToken))
}

// List returns every token owned by the calling user. The response contains
// metadata only; raw tokens are never embedded even when the deployment
// supports reveal. 500 if the repository is unavailable.
func (h *ClientTokenHandler) List(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())

	tokens, err := h.svc.ListTokens(r.Context(), p.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}

	resp := dto.ListClientTokensResponse{Tokens: make([]dto.ClientTokenMetaResponse, 0, len(tokens))}
	for _, detail := range tokens {
		resp.Tokens = append(resp.Tokens, dto.ClientTokenMetaFromDetail(detail))
	}
	writeJSON(w, http.StatusOK, resp)
}

// Revoke marks the token as revoked. 200 on success, 400 when token_id is
// missing, 403 when the calling user is not the owner, 404 when the token
// does not exist, 500 otherwise. Revocation is durable; a previously valid
// token will fail every subsequent LookupByRawToken.
func (h *ClientTokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	tokenID := r.PathValue("token_id")
	if tokenID == "" {
		writeError(w, http.StatusBadRequest, "token_id is required")
		return
	}

	if err := h.svc.RevokeToken(r.Context(), tokenID, p.UserID); err != nil {
		switch {
		case errors.Is(err, appClientToken.ErrUnauthorized):
			writeError(w, http.StatusForbidden, "cannot revoke another user's token")
		case errors.Is(err, clienttoken.ErrNotFound):
			writeError(w, http.StatusNotFound, "token not found")
		default:
			writeError(w, http.StatusInternalServerError, "failed to revoke token")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
