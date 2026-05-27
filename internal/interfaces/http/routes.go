// Package http provides the HTTP server and routing for the kish API.
package http

import (
	"net/http"

	"github.com/AFDEAPAC/kish/internal/interfaces/http/handler"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// RegisterRoutes registers all API routes on the provided ServeMux using
// Go 1.22+ method+pattern routing.
//
// Auth middleware is not applied here. The caller is expected to wrap the
// finished mux with WrapWithAuth so every request has a principal attached
// before any handler runs. Per-route guards (RequireAuthenticated,
// RequireAdmin, RequireJWT) decide whether an authenticated principal is
// required and which authentication methods are accepted.
//
// Route prefixes:
//   - /api/auth/        login, refresh, logout, current principal
//   - /api/users/       admin-only user management
//   - /api/me/          current-user profile and client-token management
//   - /api/v1/          TestCase and Artifact APIs
//
// GET /api/testcases/{id} is kept as a legacy backward-compatible read.
// Anonymous callers see public-published TestCases only; the visibility
// filter lives in the handler/service layer.
func RegisterRoutes(
	mux *http.ServeMux,
	health *handler.HealthHandler,
	tc *handler.TestCaseHandler,
	art *handler.ArtifactHandler,
	authH *handler.AuthHandler,
	userH *handler.UserHandler,
	meH *handler.MeHandler,
	ctH *handler.ClientTokenHandler,
) {
	mux.HandleFunc("GET /healthz", health.Check)

	// Auth endpoints — login/refresh are public; logout/me require authentication.
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
	mux.HandleFunc("GET /api/me/client-tokens/{token_id}", middleware.RequireJWT(ctH.Reveal))
	mux.HandleFunc("DELETE /api/me/client-tokens/{token_id}", middleware.RequireJWT(ctH.Revoke))

	// TestCase metadata API (v1).
	// List + GetV1 accept anonymous requests but filter to public-published when
	// caller is anonymous. Create / Update / Publish require authentication and
	// enforce ownership inside the handler/service layer.
	mux.HandleFunc("GET /api/v1/testcases", tc.List)
	mux.HandleFunc("POST /api/v1/testcases", tc.CreateV1)
	mux.HandleFunc("GET /api/v1/testcases/{case_id}", tc.GetV1)
	mux.HandleFunc("PATCH /api/v1/testcases/{case_id}", middleware.RequireAuthenticated(tc.Update))
	mux.HandleFunc("POST /api/v1/testcases/{case_id}/publish", middleware.RequireAuthenticated(tc.Publish))
	mux.HandleFunc("DELETE /api/v1/testcases/{case_id}", middleware.RequireAuthenticated(tc.Delete))

	mux.HandleFunc("GET /api/testcases/{id}", tc.Get)

	// Artifact API (v1) — reads enforce visibility against the parent TestCase;
	// writes require authentication (checked inside the handler).
	mux.HandleFunc("GET /api/v1/testcases/{case_id}/artifacts", art.List)
	mux.HandleFunc("PUT /api/v1/testcases/{case_id}/artifacts/{artifact_name}", art.Put)
	mux.HandleFunc("GET /api/v1/testcases/{case_id}/artifacts/{artifact_name}", art.Get)
	mux.HandleFunc("DELETE /api/v1/testcases/{case_id}/artifacts/{artifact_name}", art.Delete)
}

// WrapWithAuth wraps the mux with the auth middleware so every request has
// a principal attached to its context before reaching any handler. Per-route
// guards registered by RegisterRoutes rely on this wrapping; calling
// RegisterRoutes without also wrapping with WrapWithAuth (or an equivalent
// middleware that populates the request context) will cause guards to reject
// every request as unauthenticated.
func WrapWithAuth(mux http.Handler, authMiddleware func(http.Handler) http.Handler) http.Handler {
	return authMiddleware(mux)
}
