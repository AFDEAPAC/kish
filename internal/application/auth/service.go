// Package auth provides the application-layer service for authentication.
//
// Service covers credential-based login (with password verification and token
// issuance), refresh-token rotation, and logout (session revocation). Refresh
// tokens are opaque random strings whose SHA-256 hash is stored as a session
// record; the raw value is returned to the caller exactly once and must be
// treated as a bearer credential.
//
// Principal resolution for incoming requests (JWT or client token) is the
// responsibility of the HTTP auth middleware in
// internal/interfaces/http/middleware; this package never inspects request
// metadata or transport details.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/session"
	"github.com/AFDEAPAC/kish/internal/domain/user"
)

// ErrInvalidCredentials is the single sentinel returned for any login failure
// caused by an unknown email, a wrong password, or a disabled account. The
// three cases are deliberately collapsed so callers cannot use error
// distinctions to enumerate users or probe account status.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrInvalidRefreshToken is returned when the provided refresh token is
// expired, revoked, not found, or belongs to a user that has since been
// disabled. As with ErrInvalidCredentials the cases are collapsed so refresh
// failures do not leak account state.
var ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")

// LoginResult carries the tokens and user info returned by a successful login.
//
// User.PasswordHash is always cleared before LoginResult leaves Service. The
// caller is expected to relay RefreshToken to the client exactly once; the
// raw value cannot be recovered after Logout or rotation.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	// ExpiresIn is the access-token lifetime in seconds advertised to the
	// client. The value reflects the JWT TTL configured for the issuer; the
	// authoritative expiry remains the JWT exp claim, and clients must not
	// extend the token past it even if ExpiresIn is larger due to a stale
	// build.
	ExpiresIn int64
	User      *user.User
}

// RefreshResult carries the rotated access and refresh tokens. The previous
// refresh token is already revoked by the time RefreshResult is returned, so
// the new RefreshToken is the only valid credential for subsequent refreshes.
type RefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// PasswordVerifier is the authentication use case's credential verification
// port. Implementations hide password hashing details from application logic.
type PasswordVerifier interface {
	VerifyPassword(plaintext, hash string) (bool, error)
}

// AccessTokenIssuer signs short-lived access tokens for authenticated users.
type AccessTokenIssuer interface {
	Issue(userID string, role user.UserRole) (string, error)
}

// OpaqueTokenCodec generates and hashes refresh tokens without exposing the
// concrete cryptographic implementation to the authentication use case.
type OpaqueTokenCodec interface {
	Generate(prefix string) (string, error)
	Hash(raw string) string
}

// Service orchestrates credential-based login and refresh-token rotation.
//
// Service is the only place inside the application layer that combines user
// lookup, password verification, JWT issuance, and refresh-session
// persistence. All ports are injected so the use case can be tested without
// a database, JWT library, or password hasher. Service performs no
// transport-level work and is safe for concurrent use as long as every
// injected port is also safe for concurrent use.
//
// Authorization model: Service trusts that the caller has not yet been
// authenticated. It is the caller's responsibility to confirm the returned
// user is allowed to access the requested resource. Service itself only
// enforces the credential invariants documented on Login and Refresh.
type Service struct {
	userRepo    user.Repository
	sessionRepo session.Repository
	hasher      PasswordVerifier
	issuer      AccessTokenIssuer
	refresh     OpaqueTokenCodec
	refreshTTL  time.Duration
}

// NewService wires Service with the ports it needs. refreshTTL controls how
// long an issued refresh token is considered valid; rotation on Refresh does
// not extend the original expiry beyond this window for the new token.
func NewService(
	userRepo user.Repository,
	sessionRepo session.Repository,
	hasher PasswordVerifier,
	issuer AccessTokenIssuer,
	refresh OpaqueTokenCodec,
	refreshTTL time.Duration,
) *Service {
	return &Service{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		hasher:      hasher,
		issuer:      issuer,
		refresh:     refresh,
		refreshTTL:  refreshTTL,
	}
}

// Login authenticates a user by email and password and issues a fresh access
// token plus refresh token.
//
// Login returns ErrInvalidCredentials when the email does not exist, the
// password does not match, or the account is disabled. Any other error
// represents an infrastructure failure (repository, hasher, issuer) and
// should not be exposed to the caller verbatim. Login respects ctx
// cancellation for repository work but the password hasher may be
// CPU-bound and not interruptible.
//
// LoginResult.User has PasswordHash cleared. LoginResult.RefreshToken is the
// raw refresh token and is shown to the client exactly once; only its
// SHA-256 hash is persisted as a session record.
func (s *Service) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	u, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("login lookup: %w", err)
	}

	if u.Status == user.StatusDisabled {
		// Treat disabled accounts as invalid credentials to avoid leaking status.
		return nil, ErrInvalidCredentials
	}

	ok, err := s.hasher.VerifyPassword(password, u.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}

	accessToken, expiresIn, err := s.issueAccessToken(u)
	if err != nil {
		return nil, err
	}

	rawRefresh, err := s.issueRefreshToken(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	u.PasswordHash = ""
	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		ExpiresIn:    expiresIn,
		User:         u,
	}, nil
}

// Refresh rotates a refresh token: it revokes the supplied token, re-checks
// the owning user is still active, and issues a fresh access/refresh pair.
//
// Refresh returns ErrInvalidRefreshToken when the token is unknown, already
// revoked, expired, or owned by a now-disabled user. Re-checking user status
// here is mandatory; otherwise a refresh token issued before DisableUser
// could keep producing access tokens indefinitely.
//
// Rotation is not transactional: revoking the old session and persisting the
// new one happen in separate repository calls. If the second call fails the
// old session is already revoked and the caller must log in again.
func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (*RefreshResult, error) {
	hash := s.refresh.Hash(rawRefreshToken)
	sess, err := s.sessionRepo.FindByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("find session: %w", err)
	}

	if !sess.IsValid(time.Now().UTC()) {
		return nil, ErrInvalidRefreshToken
	}

	// Refresh must re-check account status so a disabled user cannot extend an
	// otherwise-valid session by rotating refresh tokens.
	u, err := s.userRepo.FindByID(ctx, sess.UserID)
	if err != nil {
		return nil, fmt.Errorf("load user for refresh: %w", err)
	}
	if u.Status == user.StatusDisabled {
		return nil, ErrInvalidRefreshToken
	}

	// Revoke the old session (rotation).
	if err := s.sessionRepo.Revoke(ctx, sess.ID); err != nil {
		return nil, fmt.Errorf("revoke old session: %w", err)
	}

	accessToken, expiresIn, err := s.issueAccessToken(u)
	if err != nil {
		return nil, err
	}

	rawNewRefresh, err := s.issueRefreshToken(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	return &RefreshResult{
		AccessToken:  accessToken,
		RefreshToken: rawNewRefresh,
		ExpiresIn:    expiresIn,
	}, nil
}

// Logout revokes the session identified by the provided raw refresh token.
//
// Logout is idempotent: if the session is already gone (unknown hash, or
// already revoked) Logout returns nil so a client retrying a logout request
// does not see a confusing error. Any non-nil error represents an
// infrastructure failure; callers must not block sign-out on it.
func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	hash := s.refresh.Hash(rawRefreshToken)
	sess, err := s.sessionRepo.FindByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			// Already gone; treat as success.
			return nil
		}
		return fmt.Errorf("find session for logout: %w", err)
	}
	return s.sessionRepo.Revoke(ctx, sess.ID)
}

func (s *Service) issueAccessToken(u *user.User) (string, int64, error) {
	token, err := s.issuer.Issue(u.ID, u.Role)
	if err != nil {
		return "", 0, fmt.Errorf("issue access token: %w", err)
	}
	// ExpiresIn is advertised to clients as a hint. The authoritative expiry
	// lives in the signed JWT exp claim controlled by AccessTokenIssuer. The
	// constant below mirrors the deployment default of auth.jwt_ttl=24h; if
	// the operator shortens jwt_ttl this hint will be larger than the real
	// lifetime, and clients must still respect the exp claim.
	// TODO(auth): expose the real TTL from AccessTokenIssuer so this hint
	// cannot drift from the signed exp claim.
	expiresIn := int64(24 * 60 * 60)
	return token, expiresIn, nil
}

// issueRefreshToken generates a new opaque refresh token, persists its hash,
// and returns the raw token to the caller.
func (s *Service) issueRefreshToken(ctx context.Context, userID string) (string, error) {
	rawToken, err := s.refresh.Generate("ref")
	if err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	sess := &session.Session{
		UserID:    userID,
		TokenHash: s.refresh.Hash(rawToken),
		ExpiresAt: time.Now().UTC().Add(s.refreshTTL),
	}
	if _, err := s.sessionRepo.Create(ctx, sess); err != nil {
		return "", fmt.Errorf("persist session: %w", err)
	}
	return rawToken, nil
}
