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

// EnvironmentArtifactRef is the TestCase-level reference to an uploaded
// EnvironmentSnapshot artifact. The raw JSON remains in the artifact store; this
// summary lets TestCase detail reads expose environment identity without
// scanning the artifact list.
type EnvironmentArtifactRef struct {
	Scope           environment.EnvironmentScope `json:"scope"`
	ArtifactName    string                       `json:"artifact_name"`
	SchemaVersion   string                       `json:"schema_version,omitempty"`
	EnvironmentType environment.EnvironmentType  `json:"environment_type,omitempty"`
	CollectedAt     time.Time                    `json:"collected_at,omitempty"`
	ContentType     string                       `json:"content_type,omitempty"`
	Size            int64                        `json:"size,omitempty"`
	SHA256          string                       `json:"checksum_sha256,omitempty"`
	UploadedAt      time.Time                    `json:"uploaded_at,omitempty"`
}

// TestResultArtifactRef is a TestCase-level pointer to one result artifact.
//
// A TestCase may carry multiple result references (1..N). The slice is keyed by
// ArtifactName via UpsertTestResultArtifact, so re-uploading the same name
// refreshes the metadata in place rather than producing duplicate entries.
// Once the parent TestCase is published, the whole TestResults slice is
// immutable: the application layer rejects further uploads, replacements, and
// deletions of result artifacts. The raw payload always lives in the artifact
// ObjectStore; this struct only carries metadata needed by TestCase reads.
type TestResultArtifactRef struct {
	ArtifactName string    `json:"artifact_name"`
	ContentType  string    `json:"content_type,omitempty"`
	Size         int64     `json:"size,omitempty"`
	SHA256       string    `json:"checksum_sha256,omitempty"`
	UploadedAt   time.Time `json:"uploaded_at,omitempty"`
}

// TestScriptArtifactRef points at a script artifact associated with the run.
// Multiple scripts may be attached; uploading the same artifact_name updates the
// reference metadata rather than adding a duplicate entry.
type TestScriptArtifactRef struct {
	ArtifactName string    `json:"artifact_name"`
	ContentType  string    `json:"content_type,omitempty"`
	Size         int64     `json:"size,omitempty"`
	SHA256       string    `json:"checksum_sha256,omitempty"`
	UploadedAt   time.Time `json:"uploaded_at,omitempty"`
}

// TestCase is the aggregate root that represents a single test run.
//
// Starting from the artifact API, a TestCase holds metadata plus first-class
// references to canonical artifacts. File content is stored via the artifact
// ObjectStore and referenced through the artifact metadata collection.
//
// Canonical result artifacts live in TestResults as an ordered, name-keyed
// slice managed exclusively through UpsertTestResultArtifact and
// RemoveTestResultArtifact. The inline ResultArtifact / ScriptArtifacts
// fields may still be present on documents created before the artifact API
// was introduced; new code must not write them.
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

	// Environments are first-class references to uploaded environment snapshots.
	Environments []EnvironmentArtifactRef `json:"environments,omitempty"`

	// DefaultEnvironmentScope identifies the environment shown by default in
	// detail views. It is set to execution when an execution snapshot exists.
	DefaultEnvironmentScope environment.EnvironmentScope `json:"default_environment_scope,omitempty"`

	// TestResults is the canonical, append-ordered list of result artifact
	// references uploaded through the v1 artifact API. Entries are keyed by
	// ArtifactName (see UpsertTestResultArtifact / RemoveTestResultArtifact)
	// and become immutable once Status=Published. Read-only fallback from the
	// legacy singular `test_result` document field happens in the MongoDB
	// adapter; new code must read and write through TestResults only.
	TestResults []TestResultArtifactRef `json:"test_results,omitempty"`

	// TestScripts are script artifact references for v1 uploads.
	TestScripts []TestScriptArtifactRef `json:"test_scripts,omitempty"`

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

// SetEnvironmentArtifact records or replaces the environment artifact reference
// for its scope. Execution snapshots become the default environment because the
// execution environment is the primary context for interpreting a result.
func (t *TestCase) SetEnvironmentArtifact(ref EnvironmentArtifactRef) {
	for i := range t.Environments {
		if t.Environments[i].Scope == ref.Scope {
			t.Environments[i] = ref
			t.refreshDefaultEnvironmentScope()
			return
		}
	}
	t.Environments = append(t.Environments, ref)
	t.refreshDefaultEnvironmentScope()
}

// UpsertTestResultArtifact records ref in TestResults, keyed by ArtifactName.
//
// The first upload of a name appends, preserving upload order. Re-uploading
// the same name replaces the existing entry in place so its index (the
// observable order on the API surface) is stable across overwrites. Callers
// must not invoke this on a published TestCase; the application layer is
// responsible for enforcing that invariant before mutating the aggregate.
func (t *TestCase) UpsertTestResultArtifact(ref TestResultArtifactRef) {
	for i := range t.TestResults {
		if t.TestResults[i].ArtifactName == ref.ArtifactName {
			t.TestResults[i] = ref
			return
		}
	}
	t.TestResults = append(t.TestResults, ref)
}

// RemoveTestResultArtifact unlinks the result reference identified by
// artifactName from TestResults.
//
// It is the in-aggregate counterpart of an artifact delete: the artifact
// service calls it after removing the underlying ObjectStore content and
// metadata so the TestCase no longer points at vanished data. The boolean
// return lets callers skip a redundant repository write when no entry
// matched (e.g. legacy documents that never linked a result). The application
// layer must only invoke this while Status=Draft; published TestCases have
// immutable result sets.
func (t *TestCase) RemoveTestResultArtifact(artifactName string) bool {
	for i := range t.TestResults {
		if t.TestResults[i].ArtifactName == artifactName {
			t.TestResults = append(t.TestResults[:i], t.TestResults[i+1:]...)
			return true
		}
	}
	return false
}

// UpsertTestScriptArtifact appends a script reference or updates the existing
// reference for the same artifact name.
func (t *TestCase) UpsertTestScriptArtifact(ref TestScriptArtifactRef) {
	for i := range t.TestScripts {
		if t.TestScripts[i].ArtifactName == ref.ArtifactName {
			t.TestScripts[i] = ref
			return
		}
	}
	t.TestScripts = append(t.TestScripts, ref)
}

// HasExecutionEnvironment reports whether the TestCase has a canonical
// execution environment reference.
func (t *TestCase) HasExecutionEnvironment() bool {
	for _, ref := range t.Environments {
		if ref.Scope == environment.ScopeExecution {
			return true
		}
	}
	return false
}

// HasTestResult reports whether the TestCase has at least one canonical result
// artifact reference in TestResults.
//
// Legacy inline ResultArtifact data is intentionally ignored: callers asking
// "does this TestCase have a result on the artifact API surface" want a clean
// "no" for pre-artifact-API documents so they can be re-uploaded before being
// treated as complete.
func (t *TestCase) HasTestResult() bool {
	return len(t.TestResults) > 0
}

func (t *TestCase) refreshDefaultEnvironmentScope() {
	if t.HasExecutionEnvironment() {
		t.DefaultEnvironmentScope = environment.ScopeExecution
		return
	}
	t.DefaultEnvironmentScope = ""
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
