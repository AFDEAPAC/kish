package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
)

const collectionTestCases = "test_cases"

// TestCaseRepository implements domain/testcase.Repository using MongoDB.
//
// Documents are stored with _id mapped to the TestCase ID string.
// The EnvironmentSnapshot is stored as a JSON string rather than an embedded
// BSON document because its Data map contains dotted keys (e.g. "os.name")
// which MongoDB interprets as nested path separators.
type TestCaseRepository struct {
	col *mongo.Collection
}

// NewTestCaseRepository constructs a TestCaseRepository and ensures the required
// indexes exist. Index creation is idempotent and safe to call on every startup.
func NewTestCaseRepository(ctx context.Context, db *mongo.Database) (*TestCaseRepository, error) {
	col := db.Collection(collectionTestCases)
	if err := ensureIndexes(ctx, col); err != nil {
		return nil, fmt.Errorf("failed to ensure test_cases indexes: %w", err)
	}
	return &TestCaseRepository{col: col}, nil
}

// Create inserts a new TestCase document. Returns an error if the ID already exists.
func (r *TestCaseRepository) Create(ctx context.Context, tc *testcase.TestCase) error {
	doc, err := toDocument(tc)
	if err != nil {
		return err
	}
	if _, err := r.col.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("testcase insert: %w", err)
	}
	return nil
}

// Update replaces the content fields of an existing TestCase.
// CreatedAt is preserved from the stored document.
// Returns testcase.ErrNotFound if the ID does not exist.
func (r *TestCaseRepository) Update(ctx context.Context, id string, tc *testcase.TestCase) error {
	updateFields, err := toUpdateFields(tc)
	if err != nil {
		return err
	}

	filter := bson.D{{Key: "_id", Value: id}}
	update := bson.D{{Key: "$set", Value: updateFields}}

	result, err := r.col.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("testcase update: %w", err)
	}
	if result.MatchedCount == 0 {
		return testcase.ErrNotFound
	}
	return nil
}

// FindByID retrieves a TestCase by its string ID.
// Returns testcase.ErrNotFound if the document does not exist.
func (r *TestCaseRepository) FindByID(ctx context.Context, id string) (*testcase.TestCase, error) {
	filter := bson.D{{Key: "_id", Value: id}}
	var doc testCaseDocument
	if err := r.col.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, testcase.ErrNotFound
		}
		return nil, fmt.Errorf("testcase find: %w", err)
	}
	return fromDocument(doc)
}

// ensureIndexes creates the required indexes if they do not exist.
func ensureIndexes(ctx context.Context, col *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "created_at", Value: 1}}},
		{Keys: bson.D{{Key: "updated_at", Value: 1}}},
		{Keys: bson.D{{Key: "test_type", Value: 1}}},
	}
	opts := options.CreateIndexes()
	_, err := col.Indexes().CreateMany(ctx, indexes, opts)
	return err
}

// testCaseDocument is the MongoDB storage representation of a TestCase.
// environment_json stores the EnvironmentSnapshot as a JSON string to avoid
// MongoDB's restriction on dotted keys in embedded documents.
type testCaseDocument struct {
	ID              string                   `bson:"_id"`
	Name            string                   `bson:"name"`
	TestType        string                   `bson:"test_type"`
	OwnerUserID     string                   `bson:"owner_user_id,omitempty"`
	EnvironmentJSON string                   `bson:"environment_json"`
	ResultArtifact  inlineArtifactDocument   `bson:"result_artifact"`
	ScriptArtifacts []inlineArtifactDocument `bson:"script_artifacts"`
	CreatedAt       time.Time                `bson:"created_at"`
	UpdatedAt       time.Time                `bson:"updated_at"`
}

// inlineArtifactDocument is the BSON representation of a testcase.Artifact
// stored inline within a TestCase document. It is distinct from artifactDocument
// in artifact_repo.go which represents the standalone artifact metadata collection.
type inlineArtifactDocument struct {
	Type        string    `bson:"type"`
	Filename    string    `bson:"filename"`
	ContentType string    `bson:"content_type"`
	SizeBytes   int64     `bson:"size_bytes"`
	SHA256      string    `bson:"sha256"`
	Content     string    `bson:"content"`
	CreatedAt   time.Time `bson:"created_at"`
}

func toDocument(tc *testcase.TestCase) (testCaseDocument, error) {
	envJSON, err := marshalEnvJSON(tc.Environment)
	if err != nil {
		return testCaseDocument{}, err
	}

	scripts := make([]inlineArtifactDocument, 0, len(tc.ScriptArtifacts))
	for _, a := range tc.ScriptArtifacts {
		scripts = append(scripts, toArtifactDocument(a))
	}

	return testCaseDocument{
		ID:              tc.ID,
		Name:            tc.Name,
		TestType:        tc.TestType,
		OwnerUserID:     tc.OwnerUserID,
		EnvironmentJSON: envJSON,
		ResultArtifact:  toArtifactDocument(tc.ResultArtifact),
		ScriptArtifacts: scripts,
		CreatedAt:       tc.CreatedAt,
		UpdatedAt:       tc.UpdatedAt,
	}, nil
}

func toUpdateFields(tc *testcase.TestCase) (bson.D, error) {
	envJSON, err := marshalEnvJSON(tc.Environment)
	if err != nil {
		return nil, err
	}

	scripts := make([]inlineArtifactDocument, 0, len(tc.ScriptArtifacts))
	for _, a := range tc.ScriptArtifacts {
		scripts = append(scripts, toArtifactDocument(a))
	}

	return bson.D{
		{Key: "name", Value: tc.Name},
		{Key: "test_type", Value: tc.TestType},
		{Key: "environment_json", Value: envJSON},
		{Key: "result_artifact", Value: toArtifactDocument(tc.ResultArtifact)},
		{Key: "script_artifacts", Value: scripts},
		{Key: "updated_at", Value: tc.UpdatedAt},
	}, nil
}

func toArtifactDocument(a testcase.Artifact) inlineArtifactDocument {
	return inlineArtifactDocument{
		Type:        string(a.Type),
		Filename:    a.Filename,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		SHA256:      a.SHA256,
		Content:     a.Content,
		CreatedAt:   a.CreatedAt,
	}
}

func fromDocument(doc testCaseDocument) (*testcase.TestCase, error) {
	env, err := unmarshalEnvJSON(doc.EnvironmentJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal environment: %w", err)
	}

	scripts := make([]testcase.Artifact, 0, len(doc.ScriptArtifacts))
	for _, a := range doc.ScriptArtifacts {
		scripts = append(scripts, fromArtifactDocument(a))
	}

	return &testcase.TestCase{
		ID:              doc.ID,
		Name:            doc.Name,
		TestType:        doc.TestType,
		OwnerUserID:     doc.OwnerUserID,
		Environment:     env,
		ResultArtifact:  fromArtifactDocument(doc.ResultArtifact),
		ScriptArtifacts: scripts,
		CreatedAt:       doc.CreatedAt,
		UpdatedAt:       doc.UpdatedAt,
	}, nil
}

func fromArtifactDocument(a inlineArtifactDocument) testcase.Artifact {
	return testcase.Artifact{
		Type:        testcase.ArtifactType(a.Type),
		Filename:    a.Filename,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		SHA256:      a.SHA256,
		Content:     a.Content,
		CreatedAt:   a.CreatedAt,
	}
}

// marshalEnvJSON serialises an EnvironmentSnapshot to a JSON string for storage.
func marshalEnvJSON(env *environment.EnvironmentSnapshot) (string, error) {
	if env == nil {
		return "", nil
	}
	b, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("marshal environment to JSON: %w", err)
	}
	return string(b), nil
}

// unmarshalEnvJSON deserialises a JSON string back to an EnvironmentSnapshot.
func unmarshalEnvJSON(s string) (*environment.EnvironmentSnapshot, error) {
	if s == "" {
		return nil, nil
	}
	var snap environment.EnvironmentSnapshot
	if err := json.Unmarshal([]byte(s), &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}
