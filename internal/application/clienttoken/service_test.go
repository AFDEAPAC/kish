package clienttoken_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appClientToken "github.com/AFDEAPAC/kish/internal/application/clienttoken"
	"github.com/AFDEAPAC/kish/internal/domain/clienttoken"
)

// --- fake repo ---

type fakeCTRepo struct {
	tokens map[string]*clienttoken.ClientToken
	byHash map[string]*clienttoken.ClientToken
}

func newFakeCTRepo() *fakeCTRepo {
	return &fakeCTRepo{
		tokens: make(map[string]*clienttoken.ClientToken),
		byHash: make(map[string]*clienttoken.ClientToken),
	}
}

func (r *fakeCTRepo) Create(_ context.Context, t *clienttoken.ClientToken) (*clienttoken.ClientToken, error) {
	if t.ID == "" {
		t.ID = "ctk_" + t.TokenHash[:8]
	}
	r.tokens[t.ID] = t
	r.byHash[t.TokenHash] = t
	return t, nil
}
func (r *fakeCTRepo) FindByID(_ context.Context, id string) (*clienttoken.ClientToken, error) {
	t, ok := r.tokens[id]
	if !ok {
		return nil, clienttoken.ErrNotFound
	}
	return t, nil
}
func (r *fakeCTRepo) FindByTokenHash(_ context.Context, hash string) (*clienttoken.ClientToken, error) {
	t, ok := r.byHash[hash]
	if !ok {
		return nil, clienttoken.ErrNotFound
	}
	return t, nil
}
func (r *fakeCTRepo) ListByUserID(_ context.Context, userID string) ([]*clienttoken.ClientToken, error) {
	var out []*clienttoken.ClientToken
	for _, t := range r.tokens {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (r *fakeCTRepo) Revoke(_ context.Context, id string) error {
	t, ok := r.tokens[id]
	if !ok {
		return clienttoken.ErrNotFound
	}
	now := time.Now()
	t.RevokedAt = &now
	return nil
}

// --- tests ---

func newCTService() (*appClientToken.Service, *fakeCTRepo) {
	repo := newFakeCTRepo()
	return appClientToken.NewService(repo, "kish"), repo
}

type fakeCipher struct{}

func (fakeCipher) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (fakeCipher) Decrypt(encoded string) (string, error) {
	if len(encoded) < 4 {
		return "", errors.New("bad cipher text")
	}
	return encoded[4:], nil
}

func newRevealCTService() (*appClientToken.Service, *fakeCTRepo) {
	repo := newFakeCTRepo()
	return appClientToken.NewService(repo, "kish", fakeCipher{}), repo
}

func expiresInFuture() *time.Time {
	t := time.Now().Add(24 * time.Hour)
	return &t
}

func TestCreateToken_Success(t *testing.T) {
	svc, _ := newCTService()
	result, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test token",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		ExpiresAt: expiresInFuture(),
		Unlimited: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RawToken == "" {
		t.Error("expected non-empty raw token")
	}
	if result.Token.TokenHash == "" {
		t.Error("expected non-empty token hash")
	}
	if result.Token.TokenHash == result.RawToken {
		t.Error("token hash must not equal raw token")
	}
}

func TestCreateToken_RawTokenHasPrefix(t *testing.T) {
	svc, _ := newCTService()
	result, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		Unlimited: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RawToken) < 5 || result.RawToken[:5] != "kish_" {
		t.Errorf("expected token to start with kish_, got: %q", result.RawToken)
	}
}

func TestCreateToken_StoresEncryptedTokenWhenCipherConfigured(t *testing.T) {
	svc, _ := newRevealCTService()
	result, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		Unlimited: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token.EncryptedToken == "" {
		t.Fatal("expected encrypted token to be stored")
	}
	if result.Token.EncryptedToken == result.RawToken {
		t.Fatal("encrypted token must not equal raw token")
	}
}

func TestRevealToken_OwnerGetsRawToken(t *testing.T) {
	svc, _ := newRevealCTService()
	result, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		Unlimited: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	revealed, err := svc.RevealToken(context.Background(), result.Token.ID, "usr1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revealed.RawToken != result.RawToken {
		t.Fatalf("expected revealed raw token to match created token")
	}
}

func TestRevealToken_NonOwnerRejected(t *testing.T) {
	svc, _ := newRevealCTService()
	result, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		Unlimited: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = func() error {
		_, err := svc.RevealToken(context.Background(), result.Token.ID, "usr2")
		return err
	}()
	if !errors.Is(err, appClientToken.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestRevealToken_LegacyTokenUnavailable(t *testing.T) {
	svc, _ := newCTService()
	result, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		Unlimited: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RevealToken(context.Background(), result.Token.ID, "usr1")
	if !errors.Is(err, appClientToken.ErrTokenContentUnavailable) {
		t.Fatalf("expected ErrTokenContentUnavailable, got %v", err)
	}
}

func TestCreateToken_InvalidScope(t *testing.T) {
	svc, _ := newCTService()
	_, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{"invalid:scope"},
		Unlimited: true,
	})
	if err == nil {
		t.Error("expected error for invalid scope")
	}
}

func TestCreateToken_MissingExpiresAt(t *testing.T) {
	svc, _ := newCTService()
	_, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		Unlimited: false,
		// ExpiresAt is nil
	})
	if err == nil {
		t.Error("expected error when expires_at is missing and unlimited is false")
	}
}

func TestRevokeToken_OwnerCanRevoke(t *testing.T) {
	svc, _ := newCTService()
	result, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		Unlimited: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.RevokeToken(context.Background(), result.Token.ID, "usr1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRevokeToken_NonOwnerRejected(t *testing.T) {
	svc, _ := newCTService()
	result, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		Unlimited: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.RevokeToken(context.Background(), result.Token.ID, "usr2")
	if !errors.Is(err, appClientToken.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestLookupByRawToken_ExpiredRejected(t *testing.T) {
	svc, repo := newCTService()
	result, err := svc.CreateToken(context.Background(), appClientToken.CreateInput{
		UserID:    "usr1",
		Name:      "test",
		Scopes:    []clienttoken.Scope{clienttoken.ScopeTestCaseWrite},
		Unlimited: false,
		ExpiresAt: func() *time.Time {
			past := time.Now().Add(-time.Hour)
			return &past
		}(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the token is in the repo.
	_ = repo

	_, err = svc.LookupByRawToken(context.Background(), result.RawToken)
	if !errors.Is(err, clienttoken.ErrNotFound) {
		t.Errorf("expected ErrNotFound for expired token, got: %v", err)
	}
}
