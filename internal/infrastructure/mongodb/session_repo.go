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

// SessionRepository implements domain/session.Repository using MongoDB.
type SessionRepository struct {
	col *mongo.Collection
}

// NewSessionRepository constructs a SessionRepository and ensures the required indexes exist.
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

	if _, err := r.col.InsertOne(ctx, s); err != nil {
		return nil, fmt.Errorf("session insert: %w", err)
	}
	return s, nil
}

// FindByTokenHash returns the session whose token_hash matches hash, or session.ErrNotFound.
func (r *SessionRepository) FindByTokenHash(ctx context.Context, hash string) (*session.Session, error) {
	filter := bson.D{{Key: "token_hash", Value: hash}}
	var s session.Session
	if err := r.col.FindOne(ctx, filter).Decode(&s); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, session.ErrNotFound
		}
		return nil, fmt.Errorf("session find: %w", err)
	}
	return &s, nil
}

// Revoke marks the session identified by id as revoked.
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
