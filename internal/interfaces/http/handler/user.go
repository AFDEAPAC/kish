package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	appUser "github.com/AFDEAPAC/kish/internal/application/user"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
)

// UserHandler adapts the user application service into the admin-only
// /api/users endpoints.
//
// Every route handled here is registered behind RequireAdmin in routes.go,
// which enforces JWT-only authentication and the admin role. The handler
// trusts that precondition and never re-verifies it.
//
// Domain/application errors map to HTTP statuses as follows:
//   - user.ErrEmailConflict        -> 409 (Create)
//   - appUser.ErrLastAdmin         -> 409 (Update, Disable)
//   - user.ErrNotFound             -> 404
//   - validation errors (wrapped)  -> 400
//   - anything else                -> 500
type UserHandler struct {
	svc *appUser.Service
}

// NewUserHandler wires UserHandler with the user application service.
func NewUserHandler(svc *appUser.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

// Create provisions a new active user. 201 with the created user on
// success. 409 when the email is already taken. 400 for validation errors
// (invalid role, short password). The password field is plaintext over TLS
// and must not be logged anywhere downstream.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	created, err := h.svc.CreateUser(r.Context(), appUser.CreateInput{
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		Role:        req.Role,
	})
	if err != nil {
		if errors.Is(err, user.ErrEmailConflict) {
			writeError(w, http.StatusConflict, "email already exists")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, dto.UserFromDomain(created))
}

// List returns every user (admin view). 200 with the full list (password
// hashes already cleared by the application layer). 500 if the repository
// is unavailable. No pagination today; the user collection is expected to
// stay small.
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	resp := dto.ListUsersResponse{Users: make([]dto.UserResponse, 0, len(users))}
	for _, u := range users {
		resp.Users = append(resp.Users, dto.UserFromDomain(u))
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get returns a single user by id. 200 on success, 400 when user_id is
// missing, 404 when the user does not exist, 500 otherwise.
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("user_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	u, err := h.svc.GetUser(r.Context(), id)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	writeJSON(w, http.StatusOK, dto.UserFromDomain(u))
}

// Update applies a non-zero patch to the user identified by user_id. 200
// on success. 409 (ErrLastAdmin) when the patch would demote or disable
// the last admin. 404 when the user does not exist. 400 for malformed
// bodies or invalid field values. Password is never changed through this
// route; admins reset passwords via a separate flow (not yet implemented).
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("user_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	var req dto.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	input := user.UpdateInput{
		DisplayName: req.DisplayName,
		Role:        req.Role,
		Status:      req.Status,
	}

	updated, err := h.svc.UpdateUser(r.Context(), id, input)
	if err != nil {
		switch {
		case errors.Is(err, appUser.ErrLastAdmin):
			writeError(w, http.StatusConflict, "cannot demote or disable the last admin")
		case errors.Is(err, user.ErrNotFound):
			writeError(w, http.StatusNotFound, "user not found")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, dto.UserFromDomain(updated))
}

// Disable soft-disables the user identified by user_id. 200 on success
// (the response body confirms the disabled state). 409 when disabling
// would leave the system with no admin. 404 when the user does not exist.
// Disable does NOT revoke the user's refresh sessions; see
// appUser.Service.DisableUser for the rationale and the follow-up that
// admins must perform when an immediate hard sign-out is required.
func (h *UserHandler) Disable(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("user_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	_, err := h.svc.DisableUser(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, appUser.ErrLastAdmin):
			writeError(w, http.StatusConflict, "cannot disable the last admin")
		case errors.Is(err, user.ErrNotFound):
			writeError(w, http.StatusNotFound, "user not found")
		default:
			writeError(w, http.StatusInternalServerError, "failed to disable user")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}
