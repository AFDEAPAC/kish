package bootstrap_test

import (
	"context"
	"testing"

	appBootstrap "github.com/AFDEAPAC/kish/internal/application/bootstrap"
	"github.com/AFDEAPAC/kish/internal/domain/user"
)

// --- fake repo ---

type fakeBootstrapUserRepo struct {
	users  map[string]*user.User
	emails map[string]*user.User
	admins int64
}

func newFakeBootstrapUserRepo() *fakeBootstrapUserRepo {
	return &fakeBootstrapUserRepo{
		users:  make(map[string]*user.User),
		emails: make(map[string]*user.User),
	}
}
func (r *fakeBootstrapUserRepo) Create(_ context.Context, u *user.User) (*user.User, error) {
	if _, ok := r.emails[u.Email]; ok {
		return nil, user.ErrEmailConflict
	}
	if u.ID == "" {
		u.ID = "usr_" + u.Email
	}
	r.users[u.ID] = u
	r.emails[u.Email] = u
	if u.Role == user.RoleAdmin {
		r.admins++
	}
	return u, nil
}
func (r *fakeBootstrapUserRepo) FindByID(_ context.Context, id string) (*user.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}
func (r *fakeBootstrapUserRepo) FindByEmail(_ context.Context, email string) (*user.User, error) {
	u, ok := r.emails[email]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}
func (r *fakeBootstrapUserRepo) Update(_ context.Context, id string, _ user.UpdateInput) (*user.User, error) {
	return r.users[id], nil
}
func (r *fakeBootstrapUserRepo) List(_ context.Context) ([]*user.User, error) { return nil, nil }
func (r *fakeBootstrapUserRepo) CountByRole(_ context.Context, role user.UserRole) (int64, error) {
	if role == user.RoleAdmin {
		return r.admins, nil
	}
	return 0, nil
}
func (r *fakeBootstrapUserRepo) UpdatePasswordHash(_ context.Context, _, _ string) error {
	return nil
}

// --- tests ---

type fakePasswordHasher struct{}

func (fakePasswordHasher) Hash(plaintext string) (string, error) { return "hash:" + plaintext, nil }

func TestBootstrap_CreatesAdminWhenNoneExists(t *testing.T) {
	repo := newFakeBootstrapUserRepo()
	hasher := fakePasswordHasher{}
	options := appBootstrap.Options{
		Enabled:          true,
		AdminEmail:       "admin@example.com",
		AdminPassword:    "admin1234",
		AdminDisplayName: "Test Admin",
	}
	svc := appBootstrap.NewService(repo, hasher, options, 8)
	svc.Run(context.Background())

	u, err := repo.FindByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("expected admin to be created: %v", err)
	}
	if u.Role != user.RoleAdmin {
		t.Errorf("expected role=admin, got: %q", u.Role)
	}
	if u.PasswordHash == "" {
		t.Error("expected non-empty password hash")
	}
}

func TestBootstrap_NoopWhenAdminExists(t *testing.T) {
	repo := newFakeBootstrapUserRepo()
	hasher := fakePasswordHasher{}

	// Pre-create an admin.
	hash, _ := hasher.Hash("existing1234")
	_, _ = repo.Create(context.Background(), &user.User{
		Email:        "existing@example.com",
		Role:         user.RoleAdmin,
		Status:       user.StatusActive,
		PasswordHash: hash,
	})

	options := appBootstrap.Options{
		Enabled:       true,
		AdminEmail:    "newadmin@example.com",
		AdminPassword: "admin1234",
	}
	svc := appBootstrap.NewService(repo, hasher, options, 8)
	svc.Run(context.Background())

	_, err := repo.FindByEmail(context.Background(), "newadmin@example.com")
	if err == nil {
		t.Error("expected no new admin to be created when one already exists")
	}
}

func TestBootstrap_DisabledDoesNotCreateAdmin(t *testing.T) {
	repo := newFakeBootstrapUserRepo()
	hasher := fakePasswordHasher{}
	options := appBootstrap.Options{
		Enabled:       false,
		AdminEmail:    "admin@example.com",
		AdminPassword: "admin1234",
	}
	svc := appBootstrap.NewService(repo, hasher, options, 8)
	svc.Run(context.Background())

	_, err := repo.FindByEmail(context.Background(), "admin@example.com")
	if err == nil {
		t.Error("expected no admin to be created when bootstrap is disabled")
	}
}

func TestBootstrap_SecondRunIsNoop(t *testing.T) {
	repo := newFakeBootstrapUserRepo()
	hasher := fakePasswordHasher{}
	options := appBootstrap.Options{
		Enabled:       true,
		AdminEmail:    "admin@example.com",
		AdminPassword: "admin1234",
	}
	svc := appBootstrap.NewService(repo, hasher, options, 8)
	svc.Run(context.Background())
	// Second run should be a no-op (admin exists).
	svc.Run(context.Background())

	count, _ := repo.CountByRole(context.Background(), user.RoleAdmin)
	if count != 1 {
		t.Errorf("expected exactly 1 admin after 2 runs, got %d", count)
	}
}
