package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	appUser "github.com/AFDEAPAC/kish/internal/application/user"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// MeHandler handles current-user self-service endpoints.
type MeHandler struct {
	svc    *appUser.Service
	hasher interface {
		Verify(plaintext, hash string) error
	}
	userRepo interface {
		FindByEmail(ctx interface{}, email string) (*user.User, error)
	}
}

// NewMeHandler constructs a MeHandler.
func NewMeHandler(svc *appUser.Service) *MeHandler {
	return &MeHandler{svc: svc}
}

// GetProfile handles GET /api/me.
func (h *MeHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	u, err := h.svc.GetUser(r.Context(), p.UserID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	writeJSON(w, http.StatusOK, dto.UserFromDomain(u))
}

// UpdateProfile handles PATCH /api/me.
func (h *MeHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())

	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "display_name is required")
		return
	}

	updated, err := h.svc.UpdateUser(r.Context(), p.UserID, user.UpdateInput{
		DisplayName: req.DisplayName,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	writeJSON(w, http.StatusOK, dto.UserFromDomain(updated))
}

// ChangePassword handles POST /api/me/password.
// The MeHandler needs access to the hasher and the full user record (including hash)
// for password verification, so it accepts a PasswordChangeService.
func (h *MeHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	p := middleware.PrincipalFromContext(r.Context())
	if err := h.svc.ChangePassword(r.Context(), p.UserID, req.CurrentPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, appUser.ErrIncorrectPassword):
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
}
