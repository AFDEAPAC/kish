package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/AFDEAPAC/kish/internal/domain/artifact"
)

const collectionArtifacts = "artifacts"

// ArtifactRepository implements domain/artifact.Repository using MongoDB.
//
// Documents are identified by the compound key (case_id, artifact_name).
// A unique index on this pair is enforced at startup. StorageKey is stored in
// MongoDB but is not returned in API responses; the application layer controls
// what fields are exposed.
type ArtifactRepository struct {
	col *mongo.Collection
}

// NewArtifactRepository constructs an ArtifactRepository and ensures required
// indexes. Index creation is idempotent and safe to call on every startup.
func NewArtifactRepository(ctx context.Context, db *mongo.Database) (*ArtifactRepository, error) {
	col := db.Collection(collectionArtifacts)
	if err := ensureArtifactIndexes(ctx, col); err != nil {
		return nil, fmt.Errorf("failed to ensure artifacts indexes: %w", err)
	}
	return &ArtifactRepository{col: col}, nil
}

// Upsert creates or replaces artifact metadata.
// On insert, created_at is set to the value in a. On update, it is preserved.
func (r *ArtifactRepository) Upsert(ctx context.Context, a *artifact.Artifact) error {
	filter := bson.D{
		{Key: "case_id", Value: a.CaseID},
		{Key: "artifact_name", Value: a.ArtifactName},
	}

	// $set updates all mutable fields; $setOnInsert preserves created_at on subsequent writes.
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "artifact_type", Value: string(a.ArtifactType)},
			{Key: "content_type", Value: a.ContentType},
			{Key: "size", Value: a.Size},
			{Key: "checksum_sha256", Value: a.SHA256},
			{Key: "storage_key", Value: a.StorageKey},
			{Key: "updated_at", Value: a.UpdatedAt},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "case_id", Value: a.CaseID},
			{Key: "artifact_name", Value: a.ArtifactName},
			{Key: "created_at", Value: a.CreatedAt},
		}},
	}

	opts := options.UpdateOne().SetUpsert(true)
	if _, err := r.col.UpdateOne(ctx, filter, update, opts); err != nil {
		return fmt.Errorf("artifact upsert: %w", err)
	}
	return nil
}

// FindOne retrieves artifact metadata by case ID and artifact name.
// Returns artifact.ErrNotFound if no matching record exists.
func (r *ArtifactRepository) FindOne(ctx context.Context, caseID, artifactName string) (*artifact.Artifact, error) {
	filter := bson.D{
		{Key: "case_id", Value: caseID},
		{Key: "artifact_name", Value: artifactName},
	}
	var doc artifactDocument
	if err := r.col.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, artifact.ErrNotFound
		}
		return nil, fmt.Errorf("artifact find: %w", err)
	}
	return fromArtifactDoc(doc), nil
}

// List returns all artifact metadata records for the given case ID ordered by artifact_name.
func (r *ArtifactRepository) List(ctx context.Context, caseID string) ([]*artifact.Artifact, error) {
	filter := bson.D{{Key: "case_id", Value: caseID}}
	opts := options.Find().SetSort(bson.D{{Key: "artifact_name", Value: 1}})

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("artifact list: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*artifact.Artifact
	for cursor.Next(ctx) {
		var doc artifactDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("artifact list decode: %w", err)
		}
		results = append(results, fromArtifactDoc(doc))
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("artifact list cursor: %w", err)
	}
	if results == nil {
		results = []*artifact.Artifact{}
	}
	return results, nil
}

// Delete removes the artifact metadata record. Returns nil if the record does not exist.
func (r *ArtifactRepository) Delete(ctx context.Context, caseID, artifactName string) error {
	filter := bson.D{
		{Key: "case_id", Value: caseID},
		{Key: "artifact_name", Value: artifactName},
	}
	if _, err := r.col.DeleteOne(ctx, filter); err != nil {
		return fmt.Errorf("artifact delete: %w", err)
	}
	return nil
}

// ensureArtifactIndexes creates the required artifact indexes if absent.
func ensureArtifactIndexes(ctx context.Context, col *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{
			// Unique compound index enforces one artifact per (case_id, artifact_name).
			Keys:    bson.D{{Key: "case_id", Value: 1}, {Key: "artifact_name", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "case_id", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: 1}}},
	}
	_, err := col.Indexes().CreateMany(ctx, indexes, options.CreateIndexes())
	return err
}

// artifactDocument is the BSON storage representation of an Artifact.
type artifactDocument struct {
	CaseID       string    `bson:"case_id"`
	ArtifactName string    `bson:"artifact_name"`
	ArtifactType string    `bson:"artifact_type"`
	ContentType  string    `bson:"content_type"`
	Size         int64     `bson:"size"`
	SHA256       string    `bson:"checksum_sha256"`
	StorageKey   string    `bson:"storage_key"`
	CreatedAt    time.Time `bson:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at"`
}

func fromArtifactDoc(doc artifactDocument) *artifact.Artifact {
	return &artifact.Artifact{
		CaseID:       doc.CaseID,
		ArtifactName: doc.ArtifactName,
		ArtifactType: artifact.ArtifactType(doc.ArtifactType),
		ContentType:  doc.ContentType,
		Size:         doc.Size,
		SHA256:       doc.SHA256,
		StorageKey:   doc.StorageKey,
		CreatedAt:    doc.CreatedAt,
		UpdatedAt:    doc.UpdatedAt,
	}
}
