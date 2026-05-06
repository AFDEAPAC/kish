package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appAuth "github.com/AFDEAPAC/kish/internal/application/auth"
	"github.com/AFDEAPAC/kish/internal/domain/session"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/infrastructure/security"
)

// --- fakes ---

type fakeAuthUserRepo struct {
	users map[string]*user.User
}

func newFakeAuthUserRepo() *fakeAuthUserRepo {
	return &fakeAuthUserRepo{users: make(map[string]*user.User)}
}
func (r *fakeAuthUserRepo) AddUser(u *user.User) { r.users[u.Email] = u; r.users[u.ID] = u }
func (r *fakeAuthUserRepo) Create(_ context.Context, u *user.User) (*user.User, error) {
	return u, nil
}
func (r *fakeAuthUserRepo) FindByID(_ context.Context, id string) (*user.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}
func (r *fakeAuthUserRepo) FindByEmail(_ context.Context, email string) (*user.User, error) {
	u, ok := r.users[email]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}
func (r *fakeAuthUserRepo) Update(_ context.Context, id string, _ user.UpdateInput) (*user.User, error) {
	return r.users[id], nil
}
func (r *fakeAuthUserRepo) List(_ context.Context) ([]*user.User, error) { return nil, nil }
func (r *fakeAuthUserRepo) CountByRole(_ context.Context, _ user.UserRole) (int64, error) {
	return 0, nil
}
func (r *fakeAuthUserRepo) UpdatePasswordHash(_ context.Context, _, _ string) error { return nil }

type fakeSessionRepo struct {
	sessions map[string]*session.Session
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: make(map[string]*session.Session)}
}
func (r *fakeSessionRepo) Create(_ context.Context, s *session.Session) (*session.Session, error) {
	if s.ID == "" {
		s.ID = "ses_" + s.TokenHash[:8]
	}
	r.sessions[s.TokenHash] = s
	return s, nil
}
func (r *fakeSessionRepo) FindByTokenHash(_ context.Context, hash string) (*session.Session, error) {
	s, ok := r.sessions[hash]
	if !ok {
		return nil, session.ErrNotFound
	}
	return s, nil
}
func (r *fakeSessionRepo) Revoke(_ context.Context, id string) error {
	for _, s := range r.sessions {
		if s.ID == id {
			now := time.Now()
			s.RevokedAt = &now
			return nil
		}
	}
	return nil
}
func (r *fakeSessionRepo) RevokeAllByUserID(_ context.Context, userID string) error {
	for _, s := range r.sessions {
		if s.UserID == userID {
			now := time.Now()
			s.RevokedAt = &now
		}
	}
	return nil
}

// --- helpers ---

func newAuthService() (*appAuth.Service, *fakeAuthUserRepo, *fakeSessionRepo) {
	userRepo := newFakeAuthUserRepo()
	sessionRepo := newFakeSessionRepo()
	hasher := security.NewBcryptHasher()
	jwtSvc := security.NewJWTService("test-secret-32chars-at-minimum!!", 24*time.Hour)
	svc := appAuth.NewService(userRepo, sessionRepo, hasher, jwtSvc, 720*time.Hour)
	return svc, userRepo, sessionRepo
}

func makeActiveUser(t *testing.T, hasher *security.BcryptHasher, email, password string, role user.UserRole) *user.User {
	t.Helper()
	hash, err := hasher.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	return &user.User{
		ID:           "usr_" + email,
		Email:        email,
		Role:         role,
		Status:       user.StatusActive,
		PasswordHash: hash,
	}
}

// --- tests ---

func TestLogin_Success(t *testing.T) {
	svc, userRepo, _ := newAuthService()
	hasher := security.NewBcryptHasher()
	u := makeActiveUser(t, hasher, "dev@example.com", "password123", user.RoleDeveloper)
	userRepo.AddUser(u)

	result, err := svc.Login(context.Background(), "dev@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if result.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if result.User.Email != "dev@example.com" {
		t.Errorf("unexpected user email: %q", result.User.Email)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, userRepo, _ := newAuthService()
	hasher := security.NewBcryptHasher()
	u := makeActiveUser(t, hasher, "dev@example.com", "password123", user.RoleDeveloper)
	userRepo.AddUser(u)

	_, err := svc.Login(context.Background(), "dev@example.com", "wrongpassword")
	if !errors.Is(err, appAuth.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestLogin_DisabledUser_Rejected(t *testing.T) {
	svc, userRepo, _ := newAuthService()
	hasher := security.NewBcryptHasher()
	u := makeActiveUser(t, hasher, "disabled@example.com", "password123", user.RoleDeveloper)
	u.Status = user.StatusDisabled
	userRepo.AddUser(u)

	_, err := svc.Login(context.Background(), "disabled@example.com", "password123")
	if !errors.Is(err, appAuth.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials for disabled user, got: %v", err)
	}
}

func TestRefresh_Success(t *testing.T) {
	svc, userRepo, _ := newAuthService()
	hasher := security.NewBcryptHasher()
	u := makeActiveUser(t, hasher, "dev@example.com", "password123", user.RoleDeveloper)
	userRepo.AddUser(u)

	loginResult, err := svc.Login(context.Background(), "dev@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}

	refreshResult, err := svc.Refresh(context.Background(), loginResult.RefreshToken)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if refreshResult.AccessToken == "" {
		t.Error("expected non-empty access token after refresh")
	}
	if refreshResult.RefreshToken == "" {
		t.Error("expected non-empty new refresh token after refresh")
	}
	// Old token should now be revoked; using it again should fail.
	_, err = svc.Refresh(context.Background(), loginResult.RefreshToken)
	if !errors.Is(err, appAuth.ErrInvalidRefreshToken) {
		t.Errorf("expected ErrInvalidRefreshToken for reused token, got: %v", err)
	}
}

func TestRefresh_RevokedToken_Rejected(t *testing.T) {
	svc, userRepo, sessionRepo := newAuthService()
	hasher := security.NewBcryptHasher()
	u := makeActiveUser(t, hasher, "dev@example.com", "password123", user.RoleDeveloper)
	userRepo.AddUser(u)

	loginResult, err := svc.Login(context.Background(), "dev@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}

	// Revoke all sessions directly.
	_ = sessionRepo.RevokeAllByUserID(context.Background(), u.ID)

	_, err = svc.Refresh(context.Background(), loginResult.RefreshToken)
	if !errors.Is(err, appAuth.ErrInvalidRefreshToken) {
		t.Errorf("expected ErrInvalidRefreshToken for revoked token, got: %v", err)
	}
}

func TestLogout_RevokesSession(t *testing.T) {
	svc, userRepo, _ := newAuthService()
	hasher := security.NewBcryptHasher()
	u := makeActiveUser(t, hasher, "dev@example.com", "password123", user.RoleDeveloper)
	userRepo.AddUser(u)

	loginResult, _ := svc.Login(context.Background(), "dev@example.com", "password123")

	if err := svc.Logout(context.Background(), loginResult.RefreshToken); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	// After logout, the refresh token should be invalid.
	_, err := svc.Refresh(context.Background(), loginResult.RefreshToken)
	if !errors.Is(err, appAuth.ErrInvalidRefreshToken) {
		t.Errorf("expected ErrInvalidRefreshToken after logout, got: %v", err)
	}
}
