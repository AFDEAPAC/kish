package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
)

const collectionTestCases = "test_cases"

// defaultListLimit is the page size applied when ListFilter.Limit is unset.
const defaultListLimit = 50

// maxListLimit caps page size to keep responses bounded.
const maxListLimit = 200

// TestCaseRepository implements domain/testcase.Repository using MongoDB.
//
// Documents are stored with _id mapped to the TestCase ID string.
// The EnvironmentSnapshot is stored as a JSON string rather than an embedded
// BSON document because its Data map contains dotted keys (e.g. "os.name")
// which MongoDB interprets as nested path separators.
//
// Legacy documents (created before status/visibility were added) are normalised
// on read to status=published, visibility=private. Combined with their empty
// owner_user_id, this keeps them visible only to admins, matching the behaviour
// of the pre-ownership era.
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

// UpdatePartial applies a metadata patch. Only non-nil patch fields are written.
// updated_at is always refreshed.
func (r *TestCaseRepository) UpdatePartial(ctx context.Context, id string, patch testcase.MetadataPatch) error {
	set := bson.D{{Key: "updated_at", Value: time.Now().UTC()}}
	if patch.Name != nil {
		set = append(set, bson.E{Key: "name", Value: *patch.Name})
	}
	if patch.Description != nil {
		set = append(set, bson.E{Key: "description", Value: *patch.Description})
	}
	if patch.TestType != nil {
		set = append(set, bson.E{Key: "test_type", Value: *patch.TestType})
	}
	if patch.Tags != nil {
		set = append(set, bson.E{Key: "tags", Value: *patch.Tags})
	}
	if patch.Visibility != nil {
		set = append(set, bson.E{Key: "visibility", Value: string(*patch.Visibility)})
	}

	filter := bson.D{{Key: "_id", Value: id}}
	update := bson.D{{Key: "$set", Value: set}}
	result, err := r.col.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("testcase update partial: %w", err)
	}
	if result.MatchedCount == 0 {
		return testcase.ErrNotFound
	}
	return nil
}

// UpdateStatus transitions the TestCase to a new status and visibility atomically.
func (r *TestCaseRepository) UpdateStatus(ctx context.Context, id string, status testcase.Status, visibility testcase.Visibility) error {
	set := bson.D{
		{Key: "status", Value: string(status)},
		{Key: "visibility", Value: string(visibility)},
		{Key: "updated_at", Value: time.Now().UTC()},
	}
	filter := bson.D{{Key: "_id", Value: id}}
	update := bson.D{{Key: "$set", Value: set}}
	result, err := r.col.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("testcase update status: %w", err)
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

// List queries TestCases according to filter. Authorization rules are
// translated into a MongoDB filter so the repository never returns records the
// caller may not see.
func (r *TestCaseRepository) List(ctx context.Context, filter testcase.ListFilter) ([]*testcase.TestCase, error) {
	mongoFilter := buildListFilter(filter)

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	cur, err := r.col.Find(ctx, mongoFilter, opts)
	if err != nil {
		return nil, fmt.Errorf("testcase list: %w", err)
	}
	defer cur.Close(ctx)

	var docs []testCaseDocument
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("testcase list decode: %w", err)
	}

	out := make([]*testcase.TestCase, 0, len(docs))
	for _, d := range docs {
		tc, err := fromDocument(d)
		if err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, nil
}

// Delete removes a TestCase document by ID.
func (r *TestCaseRepository) Delete(ctx context.Context, id string) error {
	result, err := r.col.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return fmt.Errorf("testcase delete: %w", err)
	}
	if result.DeletedCount == 0 {
		return testcase.ErrNotFound
	}
	return nil
}

// buildListFilter translates a ListFilter into a Mongo query document.
//
// Visibility rules:
//
//   - Anonymous callers: only status=published, visibility=public.
//   - Admin callers: any status/visibility.
//   - Authenticated developers: public-published OR owned (any status/visibility).
//
// Selector further restricts the candidate set:
//
//   - all     — no extra restriction beyond visibility rules above.
//   - mine    — owner_user_id == caller (no visibility relaxation needed).
//   - drafts  — status=draft owned by caller (admin sees all drafts).
//   - public  — status=published, visibility=public.
//   - private — status=published, visibility=private, owned by caller (admin sees all).
func buildListFilter(f testcase.ListFilter) bson.D {
	filter := bson.D{}

	switch f.Selector {
	case testcase.ListSelectorMine:
		if f.CallerUserID == "" {
			// Anonymous "mine" returns nothing.
			filter = append(filter, bson.E{Key: "_id", Value: bson.D{{Key: "$exists", Value: false}}})
		} else {
			filter = append(filter, bson.E{Key: "owner_user_id", Value: f.CallerUserID})
		}
	case testcase.ListSelectorDrafts:
		filter = append(filter, bson.E{Key: "status", Value: string(testcase.StatusDraft)})
		if !f.CallerIsAdmin {
			if f.CallerUserID == "" {
				filter = append(filter, bson.E{Key: "_id", Value: bson.D{{Key: "$exists", Value: false}}})
			} else {
				filter = append(filter, bson.E{Key: "owner_user_id", Value: f.CallerUserID})
			}
		}
	case testcase.ListSelectorPublic:
		filter = append(filter,
			bson.E{Key: "status", Value: string(testcase.StatusPublished)},
			bson.E{Key: "visibility", Value: string(testcase.VisibilityPublic)},
		)
	case testcase.ListSelectorPrivate:
		filter = append(filter,
			bson.E{Key: "status", Value: string(testcase.StatusPublished)},
			bson.E{Key: "visibility", Value: string(testcase.VisibilityPrivate)},
		)
		if !f.CallerIsAdmin {
			if f.CallerUserID == "" {
				filter = append(filter, bson.E{Key: "_id", Value: bson.D{{Key: "$exists", Value: false}}})
			} else {
				filter = append(filter, bson.E{Key: "owner_user_id", Value: f.CallerUserID})
			}
		}
	default: // all
		switch {
		case f.CallerIsAdmin:
			// admin sees everything; no extra filter
		case f.CallerUserID != "":
			// developer: public-published OR owned
			filter = append(filter, bson.E{Key: "$or", Value: bson.A{
				bson.D{
					{Key: "status", Value: string(testcase.StatusPublished)},
					{Key: "visibility", Value: string(testcase.VisibilityPublic)},
				},
				bson.D{{Key: "owner_user_id", Value: f.CallerUserID}},
			}})
		default:
			// anonymous: public-published only
			filter = append(filter,
				bson.E{Key: "status", Value: string(testcase.StatusPublished)},
				bson.E{Key: "visibility", Value: string(testcase.VisibilityPublic)},
			)
		}
	}

	if s := strings.TrimSpace(f.Search); s != "" {
		// Case-insensitive substring match on _id and name. We escape the
		// search term to avoid regex injection from the URL.
		escaped := regexEscape(s)
		filter = append(filter, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "_id", Value: bson.D{{Key: "$regex", Value: escaped}, {Key: "$options", Value: "i"}}}},
			bson.D{{Key: "name", Value: bson.D{{Key: "$regex", Value: escaped}, {Key: "$options", Value: "i"}}}},
		}})
	}

	return filter
}

// regexEscape escapes regex metacharacters in s so it matches as a literal substring.
func regexEscape(s string) string {
	const meta = `\.+*?()|[]{}^$`
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ensureIndexes creates the required indexes if they do not exist.
func ensureIndexes(ctx context.Context, col *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "created_at", Value: 1}}},
		{Keys: bson.D{{Key: "updated_at", Value: -1}}},
		{Keys: bson.D{{Key: "test_type", Value: 1}}},
		{Keys: bson.D{{Key: "owner_user_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "visibility", Value: 1}}},
		{Keys: bson.D{{Key: "tags", Value: 1}}},
	}
	opts := options.CreateIndexes()
	_, err := col.Indexes().CreateMany(ctx, indexes, opts)
	return err
}

// testCaseDocument is the MongoDB storage representation of a TestCase.
// environment_json stores the EnvironmentSnapshot as a JSON string to avoid
// MongoDB's restriction on dotted keys in embedded documents.
//
// status, visibility, description, and tags are written for new documents and
// missing on legacy documents; fromDocument applies safe defaults for missing values.
type testCaseDocument struct {
	ID              string                   `bson:"_id"`
	Name            string                   `bson:"name"`
	Description     string                   `bson:"description,omitempty"`
	TestType        string                   `bson:"test_type"`
	Tags            []string                 `bson:"tags,omitempty"`
	Status          string                   `bson:"status,omitempty"`
	Visibility      string                   `bson:"visibility,omitempty"`
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

	status := tc.Status
	if status == "" {
		status = testcase.StatusDraft
	}
	visibility := tc.Visibility
	if visibility == "" {
		visibility = testcase.VisibilityPrivate
	}

	return testCaseDocument{
		ID:              tc.ID,
		Name:            tc.Name,
		Description:     tc.Description,
		TestType:        tc.TestType,
		Tags:            tc.Tags,
		Status:          string(status),
		Visibility:      string(visibility),
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
		{Key: "description", Value: tc.Description},
		{Key: "test_type", Value: tc.TestType},
		{Key: "tags", Value: tc.Tags},
		{Key: "status", Value: string(tc.Status)},
		{Key: "visibility", Value: string(tc.Visibility)},
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

	// Legacy documents (created before status/visibility were added) are
	// normalised to published+private. Combined with the empty owner_user_id
	// they had at their creation, this keeps them admin-only on the API surface.
	status := testcase.Status(doc.Status)
	if !status.IsValid() {
		status = testcase.StatusPublished
	}
	visibility := testcase.Visibility(doc.Visibility)
	if !visibility.IsValid() {
		visibility = testcase.VisibilityPrivate
	}

	return &testcase.TestCase{
		ID:              doc.ID,
		Name:            doc.Name,
		Description:     doc.Description,
		TestType:        doc.TestType,
		Tags:            doc.Tags,
		Status:          status,
		Visibility:      visibility,
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
