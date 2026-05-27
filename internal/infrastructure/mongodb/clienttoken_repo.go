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

type clientTokenDocument struct {
	ID             string              `bson:"_id"`
	UserID         string              `bson:"user_id"`
	Name           string              `bson:"name"`
	TokenPrefix    string              `bson:"token_prefix"`
	TokenHash      string              `bson:"token_hash"`
	EncryptedToken string              `bson:"encrypted_token,omitempty"`
	Scopes         []clienttoken.Scope `bson:"scopes"`
	ExpiresAt      *time.Time          `bson:"expires_at,omitempty"`
	Unlimited      bool                `bson:"unlimited"`
	RevokedAt      *time.Time          `bson:"revoked_at,omitempty"`
	LastUsedAt     *time.Time          `bson:"last_used_at,omitempty"`
	CreatedAt      time.Time           `bson:"created_at"`
	UpdatedAt      time.Time           `bson:"updated_at"`
}

func clientTokenDocumentFromDomain(t *clienttoken.ClientToken) clientTokenDocument {
	return clientTokenDocument{
		ID:             t.ID,
		UserID:         t.UserID,
		Name:           t.Name,
		TokenPrefix:    t.TokenPrefix,
		TokenHash:      t.TokenHash,
		EncryptedToken: t.EncryptedToken,
		Scopes:         t.Scopes,
		ExpiresAt:      t.ExpiresAt,
		Unlimited:      t.Unlimited,
		RevokedAt:      t.RevokedAt,
		LastUsedAt:     t.LastUsedAt,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
}

func clientTokenFromDocument(doc clientTokenDocument) *clienttoken.ClientToken {
	return &clienttoken.ClientToken{
		ID:             doc.ID,
		UserID:         doc.UserID,
		Name:           doc.Name,
		TokenPrefix:    doc.TokenPrefix,
		TokenHash:      doc.TokenHash,
		EncryptedToken: doc.EncryptedToken,
		Scopes:         doc.Scopes,
		ExpiresAt:      doc.ExpiresAt,
		Unlimited:      doc.Unlimited,
		RevokedAt:      doc.RevokedAt,
		LastUsedAt:     doc.LastUsedAt,
		CreatedAt:      doc.CreatedAt,
		UpdatedAt:      doc.UpdatedAt,
	}
}

// ClientTokenRepository implements domain/clienttoken.Repository against the
// "client_tokens" collection.
//
// Index assumptions established by ensureClientTokenIndexes:
//   - token_hash for authentication lookups via FindByTokenHash. Not unique
//     in the schema; uniqueness is enforced by the random raw-token
//     generator, not by the database.
//   - user_id for ListByUserID.
//   - expires_at and created_at for sorting and future housekeeping queries.
//
// The EncryptedToken field is stored verbatim when the application layer
// supplies a non-empty value; this repository does not validate the
// ciphertext.
type ClientTokenRepository struct {
	col *mongo.Collection
}

// NewClientTokenRepository binds ClientTokenRepository to the configured
// database and ensures the indexes documented on ClientTokenRepository
// exist. ctx bounds the startup index-creation phase only.
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

	if _, err := r.col.InsertOne(ctx, clientTokenDocumentFromDomain(t)); err != nil {
		return nil, fmt.Errorf("client token insert: %w", err)
	}
	return t, nil
}

// FindByID returns the client token with the given ID, or clienttoken.ErrNotFound.
func (r *ClientTokenRepository) FindByID(ctx context.Context, id string) (*clienttoken.ClientToken, error) {
	filter := bson.D{{Key: "_id", Value: id}}
	var doc clientTokenDocument
	if err := r.col.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, clienttoken.ErrNotFound
		}
		return nil, fmt.Errorf("client token find by id: %w", err)
	}
	return clientTokenFromDocument(doc), nil
}

// FindByTokenHash returns the client token whose token_hash matches hash,
// or clienttoken.ErrNotFound when no document matches.
func (r *ClientTokenRepository) FindByTokenHash(ctx context.Context, hash string) (*clienttoken.ClientToken, error) {
	filter := bson.D{{Key: "token_hash", Value: hash}}
	var doc clientTokenDocument
	if err := r.col.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, clienttoken.ErrNotFound
		}
		return nil, fmt.Errorf("client token find by hash: %w", err)
	}
	return clientTokenFromDocument(doc), nil
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

	var docs []clientTokenDocument
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("client token list decode: %w", err)
	}
	tokens := make([]*clienttoken.ClientToken, 0, len(docs))
	for _, doc := range docs {
		tokens = append(tokens, clientTokenFromDocument(doc))
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
