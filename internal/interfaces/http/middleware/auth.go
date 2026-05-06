// Package middleware provides HTTP middleware for the kish API server.
package middleware

import (
	"context"
	"net/http"
	"strings"

	appClientToken "github.com/AFDEAPAC/kish/internal/application/clienttoken"
	"github.com/AFDEAPAC/kish/internal/domain/clienttoken"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/infrastructure/security"
)

// AuthMethod describes how the request was authenticated.
type AuthMethod string

const (
	// AuthMethodJWT means the request bears a valid JWT access token.
	AuthMethodJWT AuthMethod = "jwt"

	// AuthMethodClientToken means the request bears a valid kish_ client token.
	AuthMethodClientToken AuthMethod = "client_token"

	// AuthMethodAnonymous means no authentication token was provided.
	AuthMethodAnonymous AuthMethod = "anonymous"
)

// Principal is the resolved request-level identity attached to every request context.
type Principal struct {
	// IsAnonymous is true when no token was provided.
	IsAnonymous bool

	// UserID is the authenticated user's ID. Empty for anonymous requests.
	UserID string

	// Role is the authenticated user's role. Empty for anonymous requests.
	Role user.UserRole

	// AuthMethod identifies the token type used.
	AuthMethod AuthMethod

	// ClientTokenID is the ID of the client token, when AuthMethod is client_token.
	ClientTokenID string

	// Scopes contains the client token's scopes, when AuthMethod is client_token.
	Scopes []clienttoken.Scope
}

type contextKey struct{}

var principalKey = contextKey{}

// WithPrincipal returns a new context carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext extracts the Principal from ctx.
// Returns an anonymous Principal if none is set.
func PrincipalFromContext(ctx context.Context) Principal {
	p, ok := ctx.Value(principalKey).(Principal)
	if !ok {
		return Principal{IsAnonymous: true, AuthMethod: AuthMethodAnonymous}
	}
	return p
}

// Auth returns a middleware that resolves the request principal from the
// Authorization header and attaches it to the request context.
//
// Resolution order:
//  1. No Authorization header → anonymous principal.
//  2. Bearer token starting with "<prefix>_" → client token lookup.
//  3. Other Bearer token → JWT verification.
//
// The middleware never rejects the request; it only sets the principal.
// Individual endpoint guards (RequireAuthenticated, RequireAdmin, etc.)
// enforce access control.
func Auth(jwtSvc *security.JWTService, ctSvc *appClientToken.Service, clientTokenPrefix string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := resolveP(r, jwtSvc, ctSvc, clientTokenPrefix)
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

func resolveP(r *http.Request, jwtSvc *security.JWTService, ctSvc *appClientToken.Service, prefix string) Principal {
	raw := bearerToken(r)
	if raw == "" {
		return Principal{IsAnonymous: true, AuthMethod: AuthMethodAnonymous}
	}

	// Identify client tokens by their prefix.
	if strings.HasPrefix(raw, prefix+"_") {
		t, err := ctSvc.LookupByRawToken(r.Context(), raw)
		if err == nil {
			return Principal{
				UserID:        t.UserID,
				AuthMethod:    AuthMethodClientToken,
				ClientTokenID: t.ID,
				Scopes:        t.Scopes,
			}
		}
		// Invalid client token — fall through to JWT attempt.
	}

	// Attempt JWT verification.
	claims, err := jwtSvc.Verify(raw)
	if err == nil {
		return Principal{
			UserID:     claims.UserID,
			Role:       claims.Role,
			AuthMethod: AuthMethodJWT,
		}
	}

	// Unrecognised or invalid token — anonymous.
	return Principal{IsAnonymous: true, AuthMethod: AuthMethodAnonymous}
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return strings.TrimPrefix(auth, prefix)
}

// RequireAuthenticated wraps h and rejects anonymous requests with 401.
func RequireAuthenticated(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if PrincipalFromContext(r.Context()).IsAnonymous {
			writeUnauthorized(w, "authentication required")
			return
		}
		h(w, r)
	}
}

// RequireAdmin wraps h and rejects non-admin or non-JWT requests with 403.
// Admin endpoints must use JWT; client tokens cannot manage users.
func RequireAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := PrincipalFromContext(r.Context())
		if p.IsAnonymous {
			writeUnauthorized(w, "authentication required")
			return
		}
		if p.AuthMethod != AuthMethodJWT {
			writeForbidden(w, "admin access requires JWT authentication")
			return
		}
		if p.Role != user.RoleAdmin {
			writeForbidden(w, "admin access required")
			return
		}
		h(w, r)
	}
}

// RequireJWT wraps h and rejects non-JWT requests with 403.
// Used for endpoints that must not accept client tokens (e.g. profile update, password change).
func RequireJWT(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := PrincipalFromContext(r.Context())
		if p.IsAnonymous {
			writeUnauthorized(w, "authentication required")
			return
		}
		if p.AuthMethod != AuthMethodJWT {
			writeForbidden(w, "JWT authentication required")
			return
		}
		h(w, r)
	}
}

// RequireScope wraps h and rejects client tokens missing the given scope.
// JWT-authenticated requests always pass this check.
func RequireScope(scope clienttoken.Scope, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := PrincipalFromContext(r.Context())
		if p.IsAnonymous {
			writeUnauthorized(w, "authentication required")
			return
		}
		if p.AuthMethod == AuthMethodClientToken {
			ct := clienttoken.ClientToken{Scopes: p.Scopes}
			if !ct.HasScope(scope) {
				writeForbidden(w, "missing required scope: "+string(scope))
				return
			}
		}
		h(w, r)
	}
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

func writeForbidden(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}
