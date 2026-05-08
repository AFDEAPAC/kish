package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/clienttoken"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/infrastructure/security"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"

	appClientToken "github.com/AFDEAPAC/kish/internal/application/clienttoken"
)

// --- fake client token repo for auth middleware tests ---

type fakeMWCTRepo struct {
	tokens map[string]*clienttoken.ClientToken
}

func newFakeMWCTRepo() *fakeMWCTRepo {
	return &fakeMWCTRepo{tokens: make(map[string]*clienttoken.ClientToken)}
}
func (r *fakeMWCTRepo) Create(_ context.Context, t *clienttoken.ClientToken) (*clienttoken.ClientToken, error) {
	r.tokens[t.TokenHash] = t
	return t, nil
}
func (r *fakeMWCTRepo) FindByID(_ context.Context, id string) (*clienttoken.ClientToken, error) {
	for _, t := range r.tokens {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, clienttoken.ErrNotFound
}
func (r *fakeMWCTRepo) FindByTokenHash(_ context.Context, hash string) (*clienttoken.ClientToken, error) {
	t, ok := r.tokens[hash]
	if !ok {
		return nil, clienttoken.ErrNotFound
	}
	return t, nil
}
func (r *fakeMWCTRepo) ListByUserID(_ context.Context, _ string) ([]*clienttoken.ClientToken, error) {
	return nil, nil
}
func (r *fakeMWCTRepo) Revoke(_ context.Context, _ string) error { return nil }
func (r *fakeMWCTRepo) TouchLastUsed(_ context.Context, id string, usedAt time.Time) error {
	for _, t := range r.tokens {
		if t.ID == id {
			t.LastUsedAt = &usedAt
			t.UpdatedAt = usedAt
			return nil
		}
	}
	return clienttoken.ErrNotFound
}

// --- test helpers ---

func newJWTSvc() *security.JWTService {
	return security.NewJWTService("test-secret-minimum-32-characters!!", 24*time.Hour)
}

func newCTSvc() (*appClientToken.Service, *fakeMWCTRepo) {
	repo := newFakeMWCTRepo()
	return appClientToken.NewService(repo, "kish"), repo
}

func captureMiddleware(t *testing.T) (http.Handler, func() middleware.Principal) {
	t.Helper()
	var captured middleware.Principal
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = middleware.PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	return h, func() middleware.Principal { return captured }
}

// --- tests ---

func TestAuthMiddleware_AnonymousWhenNoHeader(t *testing.T) {
	jwtSvc := newJWTSvc()
	ctSvc, _ := newCTSvc()
	handler, getPrincipal := captureMiddleware(t)

	mw := middleware.Auth(jwtSvc, ctSvc, "kish")
	srv := httptest.NewServer(mw(handler))
	defer srv.Close()

	resp, _ := http.Get(srv.URL)
	resp.Body.Close()

	p := getPrincipal()
	if !p.IsAnonymous {
		t.Error("expected anonymous principal when no auth header")
	}
	if p.AuthMethod != middleware.AuthMethodAnonymous {
		t.Errorf("expected auth_method=anonymous, got: %v", p.AuthMethod)
	}
}

func TestAuthMiddleware_JWTParsed(t *testing.T) {
	jwtSvc := newJWTSvc()
	ctSvc, _ := newCTSvc()
	handler, getPrincipal := captureMiddleware(t)

	token, err := jwtSvc.Issue("usr_123", user.RoleDeveloper)
	if err != nil {
		t.Fatal(err)
	}

	mw := middleware.Auth(jwtSvc, ctSvc, "kish")
	srv := httptest.NewServer(mw(handler))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	p := getPrincipal()
	if p.IsAnonymous {
		t.Error("expected authenticated principal")
	}
	if p.UserID != "usr_123" {
		t.Errorf("expected user_id=usr_123, got: %q", p.UserID)
	}
	if p.AuthMethod != middleware.AuthMethodJWT {
		t.Errorf("expected auth_method=jwt, got: %v", p.AuthMethod)
	}
}

func TestAuthMiddleware_ClientTokenParsed(t *testing.T) {
	jwtSvc := newJWTSvc()
	ctSvc, repo := newCTSvc()
	handler, getPrincipal := captureMiddleware(t)

	// Create a real token through the service.
	expires := time.Now().Add(time.Hour)
	createResult, err := ctSvc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr_owner",
		Name:      "test token",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		ExpiresAt: &expires,
		Unlimited: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = repo

	mw := middleware.Auth(jwtSvc, ctSvc, "kish")
	srv := httptest.NewServer(mw(handler))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Authorization", "Bearer "+createResult.RawToken)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	p := getPrincipal()
	if p.IsAnonymous {
		t.Error("expected authenticated principal for client token")
	}
	if p.UserID != "usr_owner" {
		t.Errorf("expected user_id=usr_owner, got: %q", p.UserID)
	}
	if p.AuthMethod != middleware.AuthMethodClientToken {
		t.Errorf("expected auth_method=client_token, got: %v", p.AuthMethod)
	}
	if createResult.Token.LastUsedAt == nil {
		t.Fatal("expected client token auth to update last_used_at")
	}
}

func TestRequireAuthenticated_RejectsAnonymous(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	protected := middleware.RequireAuthenticated(inner)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	// No principal in context → anonymous.
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAdmin_RejectsNonAdmin(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	protected := middleware.RequireAdmin(inner)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	// Inject a developer JWT principal.
	p := middleware.Principal{
		UserID:     "dev1",
		Role:       user.RoleDeveloper,
		AuthMethod: middleware.AuthMethodJWT,
	}
	ctx := middleware.WithPrincipal(req.Context(), p)
	protected.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for developer on admin route, got %d", rec.Code)
	}
}

func TestRequireAdmin_RejectsClientToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	protected := middleware.RequireAdmin(inner)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	// Client token principal even with admin role should be rejected.
	p := middleware.Principal{
		UserID:     "admin1",
		Role:       user.RoleAdmin,
		AuthMethod: middleware.AuthMethodClientToken,
	}
	ctx := middleware.WithPrincipal(req.Context(), p)
	protected.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for client token on admin route, got %d", rec.Code)
	}
}

func TestRequireScope_RejectsTokenWithoutScope(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	protected := middleware.RequireScope(clienttoken.ScopeTestCaseWrite, inner)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	// Client token with only read scope.
	p := middleware.Principal{
		UserID:     "dev1",
		AuthMethod: middleware.AuthMethodClientToken,
		Scopes:     []clienttoken.Scope{clienttoken.ScopeTestCaseRead},
	}
	ctx := middleware.WithPrincipal(req.Context(), p)
	protected.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for missing testcase:write scope, got %d", rec.Code)
	}
}
