package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	appAuth "github.com/AFDEAPAC/kish/internal/application/auth"
	appUser "github.com/AFDEAPAC/kish/internal/application/user"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// AuthHandler adapts the auth and user application services into the HTTP
// authentication endpoints.
//
// Route map (registered in routes.go):
//   - POST /api/auth/login   public; issues access + refresh tokens
//   - POST /api/auth/refresh public; rotates refresh token
//   - POST /api/auth/logout  RequireAuthenticated; revokes refresh session
//   - GET  /api/auth/me      RequireAuthenticated; returns current user
//
// Domain/application errors map to HTTP statuses as follows:
//   - appAuth.ErrInvalidCredentials    -> 401 (login)
//   - appAuth.ErrInvalidRefreshToken   -> 401 (refresh)
//   - any other application error      -> 500 (intentionally opaque)
//
// 401 responses must never reveal whether the email exists or the password
// is wrong; that policy is implemented by appAuth.Service collapsing both
// into ErrInvalidCredentials.
type AuthHandler struct {
	svc     *appAuth.Service
	userSvc *appUser.Service
}

// NewAuthHandler wires the auth handler. userSvc is required for
// GET /api/auth/me, which loads the full user profile after the auth
// middleware has populated a Principal.
func NewAuthHandler(svc *appAuth.Service, userSvc *appUser.Service) *AuthHandler {
	return &AuthHandler{svc: svc, userSvc: userSvc}
}

// Login authenticates an email/password pair and returns the issued tokens.
//
// 400 when the body is not JSON or the email/password fields are missing.
// 401 (with a fixed "invalid credentials" message) when the credentials are
// wrong, unknown, or the user is disabled. 500 otherwise. The 401 message
// is intentionally constant so attackers cannot enumerate users.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	result, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, appAuth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	writeJSON(w, http.StatusOK, dto.LoginResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    result.ExpiresIn,
		User: dto.UserInfo{
			ID:          result.User.ID,
			Email:       result.User.Email,
			DisplayName: result.User.DisplayName,
			Role:        result.User.Role,
		},
	})
}

// Refresh rotates a refresh token. The old token is always revoked, even
// when issuing the new one fails partway through; clients must accept that
// a failed refresh can require re-login.
//
// 400 when the body is not JSON or refresh_token is missing. 401 when the
// token is unknown, expired, revoked, or belongs to a disabled user. 500
// otherwise.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req dto.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	result, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, appAuth.ErrInvalidRefreshToken) {
			writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}

	writeJSON(w, http.StatusOK, dto.RefreshResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    result.ExpiresIn,
	})
}

// Logout revokes the supplied refresh token. The endpoint is idempotent:
// repeating Logout for an already-revoked or unknown token still returns
// 200 so clients can safely retry sign-out on network errors.
//
// 400 when the body is not JSON or refresh_token is missing. 500 when the
// session repository is unavailable.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req dto.LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		writeError(w, http.StatusInternalServerError, "logout failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// Me returns the profile of the principal attached to the request context
// by the auth middleware. RequireAuthenticated already rejects anonymous
// callers; the IsAnonymous check below is a defence-in-depth guard against
// future middleware changes. AuthMethod (jwt or client_token) is forwarded
// from the middleware so dashboards can render method-specific UI.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	if p.IsAnonymous {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	u, err := h.userSvc.GetUser(r.Context(), p.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	writeJSON(w, http.StatusOK, dto.MeResponse{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        u.Role,
		AuthMethod:  string(p.AuthMethod),
	})
}
