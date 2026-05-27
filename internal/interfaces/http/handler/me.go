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

// MeHandler adapts the user application service into the /api/me
// self-service endpoints.
//
// Per-route authentication policy (set in routes.go):
//   - GET    /api/me           RequireAuthenticated (JWT or client token)
//   - PATCH  /api/me           RequireJWT (client tokens are read-only)
//   - POST   /api/me/password  RequireJWT (password change is interactive)
//
// All routes trust the Principal.UserID populated by the auth middleware
// and use it as the implicit subject of every operation; clients cannot
// influence which user is touched.
type MeHandler struct {
	svc *appUser.Service
}

// NewMeHandler wires MeHandler with the user application service.
func NewMeHandler(svc *appUser.Service) *MeHandler {
	return &MeHandler{svc: svc}
}

// GetProfile returns the calling user's profile. Both JWT and client-token
// callers are accepted by the route layer; PasswordHash is already cleared
// by the application service before responding. 200 on success, 404 when
// the principal points at a deleted user (rare; only happens during
// admin-initiated delete races), 500 otherwise.
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

// UpdateProfile applies the supplied display_name to the calling user.
// The endpoint is JWT-only (RequireJWT in routes.go) so client-token
// holders cannot mutate the human owner's profile. 200 on success, 400
// when display_name is missing, 500 otherwise. An inline request struct is
// used because the only field is display_name; promote to a DTO if more
// fields are added.
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

// ChangePassword updates the calling user's password.
//
// The endpoint is JWT-only so a leaked client token cannot rotate the
// owner's password and lock them out. Credential verification and hashing
// live in appUser.Service.ChangePassword; this handler only validates the
// transport shape and translates application errors.
//
// 200 on success, 400 when either field is missing or new_password fails
// policy (length), 401 when current_password does not match (
// ErrIncorrectPassword), 500 otherwise. Neither password value may be
// logged.
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
