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

	"github.com/AFDEAPAC/kish/internal/domain/user"
)

const collectionUsers = "users"

// UserRepository implements domain/user.Repository using MongoDB.
type UserRepository struct {
	col *mongo.Collection
}

// NewUserRepository constructs a UserRepository and ensures the required indexes exist.
func NewUserRepository(ctx context.Context, db *mongo.Database) (*UserRepository, error) {
	col := db.Collection(collectionUsers)
	if err := ensureUserIndexes(ctx, col); err != nil {
		return nil, fmt.Errorf("user_repo indexes: %w", err)
	}
	return &UserRepository{col: col}, nil
}

func ensureUserIndexes(ctx context.Context, col *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "role", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: 1}}},
	}
	_, err := col.Indexes().CreateMany(ctx, indexes)
	return err
}

// Create inserts a new user document. Returns user.ErrEmailConflict on duplicate email.
func (r *UserRepository) Create(ctx context.Context, u *user.User) (*user.User, error) {
	if u.ID == "" {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generate user id: %w", err)
		}
		u.ID = "usr_" + hex.EncodeToString(b)
	}
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now

	_, err := r.col.InsertOne(ctx, u)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, user.ErrEmailConflict
		}
		return nil, fmt.Errorf("user insert: %w", err)
	}
	return u, nil
}

// FindByID returns the user with the given ID, or user.ErrNotFound.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*user.User, error) {
	filter := bson.D{{Key: "_id", Value: id}}
	var u user.User
	if err := r.col.FindOne(ctx, filter).Decode(&u); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, user.ErrNotFound
		}
		return nil, fmt.Errorf("user find by id: %w", err)
	}
	return &u, nil
}

// FindByEmail returns the user with the given email, or user.ErrNotFound.
// The returned user includes PasswordHash for authentication use.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	filter := bson.D{{Key: "email", Value: email}}
	var u user.User
	if err := r.col.FindOne(ctx, filter).Decode(&u); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, user.ErrNotFound
		}
		return nil, fmt.Errorf("user find by email: %w", err)
	}
	return &u, nil
}

// Update applies the non-zero fields of input to the user identified by id.
func (r *UserRepository) Update(ctx context.Context, id string, input user.UpdateInput) (*user.User, error) {
	set := bson.D{{Key: "updated_at", Value: time.Now().UTC()}}
	if input.DisplayName != "" {
		set = append(set, bson.E{Key: "display_name", Value: input.DisplayName})
	}
	if input.Role != "" {
		set = append(set, bson.E{Key: "role", Value: string(input.Role)})
	}
	if input.Status != "" {
		set = append(set, bson.E{Key: "status", Value: string(input.Status)})
	}

	filter := bson.D{{Key: "_id", Value: id}}
	after := options.After
	opt := options.FindOneAndUpdate().SetReturnDocument(after)
	var updated user.User
	err := r.col.FindOneAndUpdate(ctx, filter, bson.D{{Key: "$set", Value: set}}, opt).Decode(&updated)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, user.ErrNotFound
		}
		return nil, fmt.Errorf("user update: %w", err)
	}
	return &updated, nil
}

// List returns all user records ordered by created_at ascending.
func (r *UserRepository) List(ctx context.Context) ([]*user.User, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})
	cur, err := r.col.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, fmt.Errorf("user list: %w", err)
	}
	defer cur.Close(ctx)

	var users []*user.User
	if err := cur.All(ctx, &users); err != nil {
		return nil, fmt.Errorf("user list decode: %w", err)
	}
	return users, nil
}

// CountByRole returns the number of users with the given role.
func (r *UserRepository) CountByRole(ctx context.Context, role user.UserRole) (int64, error) {
	filter := bson.D{{Key: "role", Value: string(role)}}
	count, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("user count by role: %w", err)
	}
	return count, nil
}

// UpdatePasswordHash replaces the stored password hash for the user identified by id.
func (r *UserRepository) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	filter := bson.D{{Key: "_id", Value: id}}
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "password_hash", Value: hash},
		{Key: "updated_at", Value: time.Now().UTC()},
	}}}
	result, err := r.col.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("update password hash: %w", err)
	}
	if result.MatchedCount == 0 {
		return user.ErrNotFound
	}
	return nil
}
