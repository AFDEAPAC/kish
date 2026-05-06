// Package testcase defines the TestCase aggregate root and its value objects.
//
// TestCase is the core domain entity for this system. File content is no longer
// stored inline; all artifact content lives in the artifact ObjectStore.
// The TestCase document holds only metadata and is created before artifacts are uploaded.
package testcase

import (
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
)

// ArtifactType classifies an inline artifact attached to a TestCase.
// Kept for backward-compatible GET responses on pre-migration documents.
type ArtifactType string

const (
	// ArtifactTypeTestResult is the required benchmark or test output.
	ArtifactTypeTestResult ArtifactType = "test_result"

	// ArtifactTypeTestScript is an optional script associated with the run.
	ArtifactTypeTestScript ArtifactType = "test_script"
)

// Artifact represents an inline artifact previously embedded in the TestCase document.
// New uploads use the separate artifact ObjectStore; this struct exists for
// backward-compatible reads of documents created before the artifact API.
type Artifact struct {
	Type        ArtifactType `json:"type"`
	Filename    string       `json:"filename"`
	ContentType string       `json:"content_type"`
	SizeBytes   int64        `json:"size_bytes"`
	SHA256      string       `json:"sha256"`
	Content     string       `json:"content"`
	CreatedAt   time.Time    `json:"created_at"`
}

// TestCase is the aggregate root that represents a single test run.
//
// Starting from the artifact API, a TestCase holds only metadata; file content
// is stored via the artifact ObjectStore and referenced through the artifact
// metadata collection. The inline ResultArtifact / ScriptArtifacts fields may
// be present on documents created before the artifact API was introduced.
type TestCase struct {
	// ID is the server-generated unique identifier (format: TC-YYYYMMDDHHMMSS-xxxx).
	ID string `json:"id"`

	// Name is an optional human-readable label for the test run.
	Name string `json:"name,omitempty"`

	// TestType classifies the test (e.g. "sglang-benchmark", "generic").
	TestType string `json:"test_type"`

	// OwnerUserID is the ID of the user who created this TestCase.
	// Empty for TestCases created before ownership was introduced.
	// TestCases with no owner are treated as admin-only resources.
	OwnerUserID string `json:"owner_user_id,omitempty"`

	// Environment is present only on pre-artifact-API documents.
	Environment *environment.EnvironmentSnapshot `json:"environment,omitempty"`

	// ResultArtifact is present only on pre-artifact-API documents.
	ResultArtifact Artifact `json:"result_artifact,omitempty"`

	// ScriptArtifacts is present only on pre-artifact-API documents.
	ScriptArtifacts []Artifact `json:"script_artifacts,omitempty"`

	// CreatedAt is when the TestCase was first created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the TestCase was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// MetadataInput carries the caller-supplied data for creating a new TestCase root.
// No artifact content is accepted at creation time; files are uploaded separately
// via the artifact API after the TestCase is created.
type MetadataInput struct {
	// Name is an optional human-readable label for the test run.
	Name string

	// TestType classifies the test. Defaults to "generic" when empty.
	TestType string

	// OwnerUserID is the ID of the authenticated user creating this TestCase.
	// Empty string is accepted for unauthenticated or pre-ownership uploads (legacy).
	OwnerUserID string
}
