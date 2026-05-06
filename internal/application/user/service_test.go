package user_test

import (
	"context"
	"errors"
	"testing"

	appUser "github.com/AFDEAPAC/kish/internal/application/user"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/infrastructure/security"
)

// --- fakes ---

type fakeUserRepo struct {
	users    map[string]*user.User
	byEmail  map[string]*user.User
	roleCounts map[user.UserRole]int64
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		users:    make(map[string]*user.User),
		byEmail:  make(map[string]*user.User),
		roleCounts: make(map[user.UserRole]int64),
	}
}

func (r *fakeUserRepo) Create(_ context.Context, u *user.User) (*user.User, error) {
	if _, ok := r.byEmail[u.Email]; ok {
		return nil, user.ErrEmailConflict
	}
	if u.ID == "" {
		u.ID = "usr_" + u.Email
	}
	r.users[u.ID] = u
	r.byEmail[u.Email] = u
	r.roleCounts[u.Role]++
	return u, nil
}
func (r *fakeUserRepo) FindByID(_ context.Context, id string) (*user.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}
func (r *fakeUserRepo) FindByEmail(_ context.Context, email string) (*user.User, error) {
	u, ok := r.byEmail[email]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}
func (r *fakeUserRepo) Update(_ context.Context, id string, input user.UpdateInput) (*user.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	if input.DisplayName != "" {
		u.DisplayName = input.DisplayName
	}
	if input.Role != "" {
		r.roleCounts[u.Role]--
		u.Role = input.Role
		r.roleCounts[u.Role]++
	}
	if input.Status != "" {
		u.Status = input.Status
	}
	return u, nil
}
func (r *fakeUserRepo) List(_ context.Context) ([]*user.User, error) {
	var out []*user.User
	for _, u := range r.users {
		out = append(out, u)
	}
	return out, nil
}
func (r *fakeUserRepo) CountByRole(_ context.Context, role user.UserRole) (int64, error) {
	return r.roleCounts[role], nil
}
func (r *fakeUserRepo) UpdatePasswordHash(_ context.Context, id, hash string) error {
	u, ok := r.users[id]
	if !ok {
		return user.ErrNotFound
	}
	u.PasswordHash = hash
	return nil
}

// --- tests ---

func newService() (*appUser.Service, *fakeUserRepo) {
	repo := newFakeUserRepo()
	hasher := security.NewBcryptHasher()
	return appUser.NewService(repo, hasher, 8), repo
}

func TestCreateUser_Success(t *testing.T) {
	svc, _ := newService()
	u, err := svc.CreateUser(context.Background(), appUser.CreateInput{
		Email:       "dev@example.com",
		DisplayName: "Developer",
		Password:    "password123",
		Role:        user.RoleDeveloper,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Email != "dev@example.com" {
		t.Errorf("unexpected email: %q", u.Email)
	}
	if u.PasswordHash != "" {
		t.Error("PasswordHash must not be returned to caller")
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	svc, _ := newService()
	in := appUser.CreateInput{Email: "dup@example.com", Password: "pass1234", Role: user.RoleDeveloper}
	if _, err := svc.CreateUser(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateUser(context.Background(), in)
	if !errors.Is(err, user.ErrEmailConflict) {
		t.Errorf("expected ErrEmailConflict, got: %v", err)
	}
}

func TestCreateUser_InvalidRole(t *testing.T) {
	svc, _ := newService()
	_, err := svc.CreateUser(context.Background(), appUser.CreateInput{
		Email:    "x@example.com",
		Password: "pass1234",
		Role:     "superadmin",
	})
	if err == nil {
		t.Error("expected error for invalid role")
	}
}

func TestCreateUser_PasswordTooShort(t *testing.T) {
	svc, _ := newService()
	_, err := svc.CreateUser(context.Background(), appUser.CreateInput{
		Email:    "x@example.com",
		Password: "short",
		Role:     user.RoleDeveloper,
	})
	if err == nil {
		t.Error("expected error for short password")
	}
}

func TestDisableUser_LastAdminProtected(t *testing.T) {
	svc, _ := newService()
	// Create a single admin.
	_, err := svc.CreateUser(context.Background(), appUser.CreateInput{
		Email:    "admin@example.com",
		Password: "admin1234",
		Role:     user.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Retrieve the stored admin.
	users, _ := svc.ListUsers(context.Background())
	adminID := users[0].ID

	_, err = svc.DisableUser(context.Background(), adminID)
	if !errors.Is(err, appUser.ErrLastAdmin) {
		t.Errorf("expected ErrLastAdmin, got: %v", err)
	}
}

func TestUpdateUser_DemoteLastAdmin_Rejected(t *testing.T) {
	svc, _ := newService()
	_, err := svc.CreateUser(context.Background(), appUser.CreateInput{
		Email:    "admin@example.com",
		Password: "admin1234",
		Role:     user.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	users, _ := svc.ListUsers(context.Background())
	adminID := users[0].ID

	_, err = svc.UpdateUser(context.Background(), adminID, user.UpdateInput{Role: user.RoleDeveloper})
	if !errors.Is(err, appUser.ErrLastAdmin) {
		t.Errorf("expected ErrLastAdmin, got: %v", err)
	}
}
