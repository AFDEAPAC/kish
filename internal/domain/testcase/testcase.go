// Package testcase defines the TestCase aggregate root and its value objects.
//
// TestCase is the core domain entity for this system. File content is no longer
// stored inline; all artifact content lives in the artifact ObjectStore.
// The TestCase document holds only metadata and is created before artifacts are uploaded.
//
// Status and Visibility together describe the publication lifecycle of a TestCase:
//
//	Draft   (status=draft,     visibility=private) — owner/admin only; result and
//	                                                  exec env can be uploaded or replaced.
//	Public  (status=published, visibility=public)  — anyone may read; result and
//	                                                  exec env are immutable; scripts
//	                                                  and supporting snapshots may be appended.
//	Private (status=published, visibility=private) — owner/admin only; same artifact
//	                                                  immutability rules as Public.
//
// Invariants enforced by the application layer:
//
//   - draft TestCases are always private
//   - only draft TestCases can be published
//   - test_type can only be edited while status=draft
//   - visibility can only be changed while status=published
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

// Status is the publication state of a TestCase.
type Status string

const (
	// StatusDraft means the TestCase is being prepared and is not visible to
	// anonymous users. Result and execution environment artifacts are still
	// replaceable while in this state.
	StatusDraft Status = "draft"

	// StatusPublished means the TestCase has been published. Result and
	// execution environment artifacts are immutable; only supporting snapshots
	// and scripts may be appended.
	StatusPublished Status = "published"
)

// IsValid reports whether s is a recognized TestCase status.
func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusPublished:
		return true
	}
	return false
}

// Visibility controls who can read a published TestCase.
// Draft TestCases are always private regardless of this field.
type Visibility string

const (
	// VisibilityPrivate means only the owner (and admins) can read the TestCase.
	VisibilityPrivate Visibility = "private"

	// VisibilityPublic means anyone, including unauthenticated users, can read
	// the TestCase. Only meaningful when status=published.
	VisibilityPublic Visibility = "public"
)

// IsValid reports whether v is a recognized TestCase visibility.
func (v Visibility) IsValid() bool {
	switch v {
	case VisibilityPrivate, VisibilityPublic:
		return true
	}
	return false
}

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

	// Description is an optional long-form description of the test run.
	Description string `json:"description,omitempty"`

	// TestType classifies the test (e.g. "sglang-benchmark", "generic").
	TestType string `json:"test_type"`

	// Tags are optional user-defined labels used for filtering and grouping.
	Tags []string `json:"tags,omitempty"`

	// Status is the publication state. New TestCases default to StatusDraft.
	Status Status `json:"status"`

	// Visibility controls anonymous read access for published TestCases.
	// Draft TestCases are always treated as private regardless of this value.
	Visibility Visibility `json:"visibility"`

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

// IsDraft reports whether the TestCase is in the draft state.
func (t *TestCase) IsDraft() bool { return t.Status == StatusDraft }

// IsPublished reports whether the TestCase is in the published state.
func (t *TestCase) IsPublished() bool { return t.Status == StatusPublished }

// IsPubliclyReadable reports whether anonymous users may read the TestCase.
// Only published-public TestCases satisfy this predicate.
func (t *TestCase) IsPubliclyReadable() bool {
	return t.IsPublished() && t.Visibility == VisibilityPublic
}

// MetadataInput carries the caller-supplied data for creating a new TestCase root.
// No artifact content is accepted at creation time; files are uploaded separately
// via the artifact API after the TestCase is created.
//
// New TestCases are always created as Draft+Private; status and visibility are
// not configurable at creation time. Use the Publish workflow to transition.
type MetadataInput struct {
	// Name is an optional human-readable label for the test run.
	Name string

	// Description is an optional long-form description.
	Description string

	// TestType classifies the test. Defaults to "generic" when empty.
	TestType string

	// Tags are optional labels for grouping and filtering.
	Tags []string

	// OwnerUserID is the ID of the authenticated user creating this TestCase.
	// Empty string is accepted for unauthenticated or pre-ownership uploads (legacy).
	OwnerUserID string
}

// MetadataPatch describes a partial metadata update for an existing TestCase.
// A nil pointer means "leave unchanged"; non-nil means "set to this value".
//
// Field-level rules enforced by the application service:
//
//   - TestType may only change while Status=Draft.
//   - Visibility may only change while Status=Published.
//   - Name, Description, and Tags may be changed in either state.
//   - Status itself is not modifiable through this patch; use the Publish workflow.
type MetadataPatch struct {
	Name        *string
	Description *string
	TestType    *string
	Tags        *[]string
	Visibility  *Visibility
}
