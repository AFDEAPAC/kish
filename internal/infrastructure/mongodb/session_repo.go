package mongodb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/AFDEAPAC/kish/internal/domain/session"
)

const collectionSessions = "sessions"

type sessionDocument struct {
	ID         string     `bson:"_id"`
	UserID     string     `bson:"user_id"`
	TokenHash  string     `bson:"token_hash"`
	ExpiresAt  time.Time  `bson:"expires_at"`
	RevokedAt  *time.Time `bson:"revoked_at,omitempty"`
	CreatedAt  time.Time  `bson:"created_at"`
	LastUsedAt *time.Time `bson:"last_used_at,omitempty"`
}

func sessionDocumentFromDomain(s *session.Session) sessionDocument {
	return sessionDocument{
		ID:         s.ID,
		UserID:     s.UserID,
		TokenHash:  s.TokenHash,
		ExpiresAt:  s.ExpiresAt,
		RevokedAt:  s.RevokedAt,
		CreatedAt:  s.CreatedAt,
		LastUsedAt: s.LastUsedAt,
	}
}

func sessionFromDocument(doc sessionDocument) *session.Session {
	return &session.Session{
		ID:         doc.ID,
		UserID:     doc.UserID,
		TokenHash:  doc.TokenHash,
		ExpiresAt:  doc.ExpiresAt,
		RevokedAt:  doc.RevokedAt,
		CreatedAt:  doc.CreatedAt,
		LastUsedAt: doc.LastUsedAt,
	}
}

// SessionRepository implements domain/session.Repository against the
// "sessions" collection.
//
// Index assumptions established by ensureSessionIndexes:
//   - token_hash for refresh-token lookup during refresh and logout.
//   - user_id to support RevokeAllByUserID on disable / hard sign-out.
//   - expires_at for housekeeping queries; the project does not currently
//     run a TTL index because revoked sessions are kept for audit.
//
// Sessions store only the SHA-256 hash of the refresh token. Like other
// repositories in this package every method is a single-document operation
// and no multi-document transaction is used.
type SessionRepository struct {
	col *mongo.Collection
}

// NewSessionRepository binds SessionRepository to the configured database
// and ensures the indexes documented on SessionRepository exist. ctx bounds
// the startup index-creation phase only.
func NewSessionRepository(ctx context.Context, db *mongo.Database) (*SessionRepository, error) {
	col := db.Collection(collectionSessions)
	if err := ensureSessionIndexes(ctx, col); err != nil {
		return nil, fmt.Errorf("session_repo indexes: %w", err)
	}
	return &SessionRepository{col: col}, nil
}

func ensureSessionIndexes(ctx context.Context, col *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "token_hash", Value: 1}}},
		{Keys: bson.D{{Key: "user_id", Value: 1}}},
		{Keys: bson.D{{Key: "expires_at", Value: 1}}},
	}
	_, err := col.Indexes().CreateMany(ctx, indexes)
	return err
}

// Create persists a new session record.
func (r *SessionRepository) Create(ctx context.Context, s *session.Session) (*session.Session, error) {
	if s.ID == "" {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generate session id: %w", err)
		}
		s.ID = "ses_" + hex.EncodeToString(b)
	}
	s.CreatedAt = time.Now().UTC()

	if _, err := r.col.InsertOne(ctx, sessionDocumentFromDomain(s)); err != nil {
		return nil, fmt.Errorf("session insert: %w", err)
	}
	return s, nil
}

// FindByTokenHash returns the session whose token_hash matches hash, or session.ErrNotFound.
func (r *SessionRepository) FindByTokenHash(ctx context.Context, hash string) (*session.Session, error) {
	filter := bson.D{{Key: "token_hash", Value: hash}}
	var doc sessionDocument
	if err := r.col.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, session.ErrNotFound
		}
		return nil, fmt.Errorf("session find: %w", err)
	}
	return sessionFromDocument(doc), nil
}

// Revoke marks the session identified by id as revoked. Missing sessions are
// treated as already revoked so logout and refresh cleanup remain idempotent.
func (r *SessionRepository) Revoke(ctx context.Context, id string) error {
	now := time.Now().UTC()
	filter := bson.D{{Key: "_id", Value: id}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "revoked_at", Value: now}}}}
	if _, err := r.col.UpdateOne(ctx, filter, update); err != nil {
		return fmt.Errorf("session revoke: %w", err)
	}
	return nil
}

// RevokeAllByUserID revokes all active sessions belonging to userID.
func (r *SessionRepository) RevokeAllByUserID(ctx context.Context, userID string) error {
	now := time.Now().UTC()
	filter := bson.D{
		{Key: "user_id", Value: userID},
		{Key: "revoked_at", Value: bson.D{{Key: "$exists", Value: false}}},
	}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "revoked_at", Value: now}}}}
	if _, err := r.col.UpdateMany(ctx, filter, update); err != nil {
		return fmt.Errorf("session revoke all: %w", err)
	}
	return nil
}
