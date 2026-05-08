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
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/AFDEAPAC/kish/internal/domain/clienttoken"
)

const collectionClientTokens = "client_tokens"

// ClientTokenRepository implements domain/clienttoken.Repository using MongoDB.
type ClientTokenRepository struct {
	col *mongo.Collection
}

// NewClientTokenRepository constructs a ClientTokenRepository and ensures indexes exist.
func NewClientTokenRepository(ctx context.Context, db *mongo.Database) (*ClientTokenRepository, error) {
	col := db.Collection(collectionClientTokens)
	if err := ensureClientTokenIndexes(ctx, col); err != nil {
		return nil, fmt.Errorf("clienttoken_repo indexes: %w", err)
	}
	return &ClientTokenRepository{col: col}, nil
}

func ensureClientTokenIndexes(ctx context.Context, col *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "token_hash", Value: 1}}},
		{Keys: bson.D{{Key: "user_id", Value: 1}}},
		{Keys: bson.D{{Key: "expires_at", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: 1}}},
	}
	_, err := col.Indexes().CreateMany(ctx, indexes)
	return err
}

// Create persists a new client token record.
func (r *ClientTokenRepository) Create(ctx context.Context, t *clienttoken.ClientToken) (*clienttoken.ClientToken, error) {
	if t.ID == "" {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generate token id: %w", err)
		}
		t.ID = "ctk_" + hex.EncodeToString(b)
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now

	if _, err := r.col.InsertOne(ctx, t); err != nil {
		return nil, fmt.Errorf("client token insert: %w", err)
	}
	return t, nil
}

// FindByID returns the client token with the given ID, or clienttoken.ErrNotFound.
func (r *ClientTokenRepository) FindByID(ctx context.Context, id string) (*clienttoken.ClientToken, error) {
	filter := bson.D{{Key: "_id", Value: id}}
	var t clienttoken.ClientToken
	if err := r.col.FindOne(ctx, filter).Decode(&t); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, clienttoken.ErrNotFound
		}
		return nil, fmt.Errorf("client token find by id: %w", err)
	}
	return &t, nil
}

// FindByTokenHash returns the client token whose token_hash matches hash.
func (r *ClientTokenRepository) FindByTokenHash(ctx context.Context, hash string) (*clienttoken.ClientToken, error) {
	filter := bson.D{{Key: "token_hash", Value: hash}}
	var t clienttoken.ClientToken
	if err := r.col.FindOne(ctx, filter).Decode(&t); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, clienttoken.ErrNotFound
		}
		return nil, fmt.Errorf("client token find by hash: %w", err)
	}
	return &t, nil
}

// ListByUserID returns all client tokens owned by the given user, ordered by created_at desc.
func (r *ClientTokenRepository) ListByUserID(ctx context.Context, userID string) ([]*clienttoken.ClientToken, error) {
	filter := bson.D{{Key: "user_id", Value: userID}}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cur, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("client token list: %w", err)
	}
	defer cur.Close(ctx)

	var tokens []*clienttoken.ClientToken
	if err := cur.All(ctx, &tokens); err != nil {
		return nil, fmt.Errorf("client token list decode: %w", err)
	}
	return tokens, nil
}

// Revoke marks the client token identified by id as revoked.
func (r *ClientTokenRepository) Revoke(ctx context.Context, id string) error {
	now := time.Now().UTC()
	filter := bson.D{{Key: "_id", Value: id}}
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "revoked_at", Value: now},
		{Key: "updated_at", Value: now},
	}}}
	result, err := r.col.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("client token revoke: %w", err)
	}
	if result.MatchedCount == 0 {
		return clienttoken.ErrNotFound
	}
	return nil
}

// TouchLastUsed records the time a client token was successfully used.
func (r *ClientTokenRepository) TouchLastUsed(ctx context.Context, id string, usedAt time.Time) error {
	filter := bson.D{{Key: "_id", Value: id}}
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "last_used_at", Value: usedAt},
		{Key: "updated_at", Value: usedAt},
	}}}
	result, err := r.col.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("client token touch last used: %w", err)
	}
	if result.MatchedCount == 0 {
		return clienttoken.ErrNotFound
	}
	return nil
}
