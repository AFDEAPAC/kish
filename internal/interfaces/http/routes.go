// Package http provides the HTTP server and routing for the kish API.
package http

import (
	"net/http"

	"github.com/AFDEAPAC/kish/internal/interfaces/http/handler"
)

// RegisterRoutes registers all API routes on the provided ServeMux.
// Go 1.22+ method+pattern routing is used so no external router is required.
//
// TestCase metadata routes are under /api/v1/.
// Artifact routes are under /api/v1/testcases/{case_id}/artifacts/.
// The legacy GET /api/testcases/{id} route is kept for backward-compatible reads.
func RegisterRoutes(
	mux *http.ServeMux,
	health *handler.HealthHandler,
	tc *handler.TestCaseHandler,
	art *handler.ArtifactHandler,
) {
	mux.HandleFunc("GET /healthz", health.Check)

	// TestCase metadata API (v1).
	mux.HandleFunc("POST /api/v1/testcases", tc.CreateV1)

	// TestCase GET kept for backward compatibility with pre-artifact-API documents.
	mux.HandleFunc("GET /api/testcases/{id}", tc.Get)

	// Artifact API (v1).
	mux.HandleFunc("GET /api/v1/testcases/{case_id}/artifacts", art.List)
	mux.HandleFunc("PUT /api/v1/testcases/{case_id}/artifacts/{artifact_name}", art.Put)
	mux.HandleFunc("GET /api/v1/testcases/{case_id}/artifacts/{artifact_name}", art.Get)
	mux.HandleFunc("DELETE /api/v1/testcases/{case_id}/artifacts/{artifact_name}", art.Delete)
}
