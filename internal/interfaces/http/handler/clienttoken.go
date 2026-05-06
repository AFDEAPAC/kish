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

// ClientTokenHandler handles client token management endpoints.
type ClientTokenHandler struct {
	svc *appClientToken.Service
}

// NewClientTokenHandler constructs a ClientTokenHandler.
func NewClientTokenHandler(svc *appClientToken.Service) *ClientTokenHandler {
	return &ClientTokenHandler{svc: svc}
}

// Create handles POST /api/me/client-tokens.
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

// List handles GET /api/me/client-tokens.
func (h *ClientTokenHandler) List(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())

	tokens, err := h.svc.ListTokens(r.Context(), p.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}

	resp := dto.ListClientTokensResponse{Tokens: make([]dto.ClientTokenMetaResponse, 0, len(tokens))}
	for _, t := range tokens {
		resp.Tokens = append(resp.Tokens, dto.ClientTokenMetaFromDomain(t))
	}
	writeJSON(w, http.StatusOK, resp)
}

// Revoke handles DELETE /api/me/client-tokens/{token_id}.
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
