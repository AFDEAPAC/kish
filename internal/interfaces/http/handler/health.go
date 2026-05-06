// Package handler contains thin HTTP handlers that translate between HTTP
// requests/responses and application service calls.
//
// Handlers must not contain business logic. They are responsible only for:
//   - decoding the request body or path parameters
//   - calling the appropriate application service
//   - encoding the service result to JSON
//   - mapping domain errors to HTTP status codes
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
)

// writeJSON serialises v to JSON and writes it with the given HTTP status code.
// Any serialisation error produces a plain-text 500 response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// At this point the header and status are already sent.
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// writeError writes a JSON error response with the given status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, dto.ErrorResponse{Error: msg})
}

// HealthHandler handles GET /healthz.
type HealthHandler struct{}

// NewHealthHandler constructs a HealthHandler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Check responds with {"status":"ok"}.
func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, dto.HealthResponse{Status: "ok"})
}
