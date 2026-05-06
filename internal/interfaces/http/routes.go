// Package http provides the HTTP server and routing for the kish API.
package http

import (
	"net/http"

	"github.com/AFDEAPAC/kish/internal/interfaces/http/handler"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// RegisterRoutes registers all API routes on the provided ServeMux.
// Go 1.22+ method+pattern routing is used so no external router is required.
//
// Auth middleware is applied globally so every handler can inspect the principal.
// Individual endpoint guards (RequireAuthenticated, RequireAdmin, RequireJWT)
// are applied per-route where needed.
//
// TestCase metadata routes are under /api/v1/.
// Artifact routes are under /api/v1/testcases/{case_id}/artifacts/.
// Auth routes are under /api/auth/.
// User management routes are under /api/users/ (admin-only).
// Current-user routes are under /api/me/.
// The legacy GET /api/testcases/{id} route is kept for backward-compatible reads.
func RegisterRoutes(
	mux *http.ServeMux,
	health *handler.HealthHandler,
	tc *handler.TestCaseHandler,
	art *handler.ArtifactHandler,
	authH *handler.AuthHandler,
	userH *handler.UserHandler,
	meH *handler.MeHandler,
	ctH *handler.ClientTokenHandler,
	authMiddleware func(http.Handler) http.Handler,
) {
	mux.HandleFunc("GET /healthz", health.Check)

	// Auth endpoints — no guard needed (login/refresh are public).
	mux.HandleFunc("POST /api/auth/login", authH.Login)
	mux.HandleFunc("POST /api/auth/refresh", authH.Refresh)
	mux.HandleFunc("POST /api/auth/logout", middleware.RequireAuthenticated(authH.Logout))
	mux.HandleFunc("GET /api/auth/me", middleware.RequireAuthenticated(authH.Me))

	// User management — admin JWT only.
	mux.HandleFunc("POST /api/users", middleware.RequireAdmin(userH.Create))
	mux.HandleFunc("GET /api/users", middleware.RequireAdmin(userH.List))
	mux.HandleFunc("GET /api/users/{user_id}", middleware.RequireAdmin(userH.Get))
	mux.HandleFunc("PATCH /api/users/{user_id}", middleware.RequireAdmin(userH.Update))
	mux.HandleFunc("DELETE /api/users/{user_id}", middleware.RequireAdmin(userH.Disable))

	// Current user — GET accepts JWT or client token; PATCH/password require JWT.
	mux.HandleFunc("GET /api/me", middleware.RequireAuthenticated(meH.GetProfile))
	mux.HandleFunc("PATCH /api/me", middleware.RequireJWT(meH.UpdateProfile))
	mux.HandleFunc("POST /api/me/password", middleware.RequireJWT(meH.ChangePassword))

	// Client token management — JWT only; client tokens cannot create more tokens.
	mux.HandleFunc("POST /api/me/client-tokens", middleware.RequireJWT(ctH.Create))
	mux.HandleFunc("GET /api/me/client-tokens", middleware.RequireJWT(ctH.List))
	mux.HandleFunc("DELETE /api/me/client-tokens/{token_id}", middleware.RequireJWT(ctH.Revoke))

	// TestCase metadata API (v1) — requires authentication (CreateV1 checks internally).
	mux.HandleFunc("POST /api/v1/testcases", tc.CreateV1)

	// TestCase GET kept for backward compatibility with pre-artifact-API documents.
	// Read is public; no auth required.
	mux.HandleFunc("GET /api/testcases/{id}", tc.Get)

	// Artifact API (v1) — reads are public; writes require authentication (checked in handler).
	mux.HandleFunc("GET /api/v1/testcases/{case_id}/artifacts", art.List)
	mux.HandleFunc("PUT /api/v1/testcases/{case_id}/artifacts/{artifact_name}", art.Put)
	mux.HandleFunc("GET /api/v1/testcases/{case_id}/artifacts/{artifact_name}", art.Get)
	mux.HandleFunc("DELETE /api/v1/testcases/{case_id}/artifacts/{artifact_name}", art.Delete)
}

// WrapWithAuth wraps the mux with the auth middleware so every request has
// a principal attached to its context before reaching any handler.
func WrapWithAuth(mux http.Handler, authMiddleware func(http.Handler) http.Handler) http.Handler {
	return authMiddleware(mux)
}
