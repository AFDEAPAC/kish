package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	appUser "github.com/AFDEAPAC/kish/internal/application/user"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
)

// UserHandler handles admin-only user management endpoints.
type UserHandler struct {
	svc *appUser.Service
}

// NewUserHandler constructs a UserHandler.
func NewUserHandler(svc *appUser.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

// Create handles POST /api/users.
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

// List handles GET /api/users.
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

// Get handles GET /api/users/{user_id}.
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

// Update handles PATCH /api/users/{user_id}.
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

// Disable handles DELETE /api/users/{user_id} (soft disable).
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
