// Package artifact defines the Artifact domain entity and related types.
//
// An Artifact represents a named file stored for a TestCase. Unlike the inline
// testcase.Artifact (which embeds content in the TestCase document), this entity
// holds only metadata; the file content lives in an ObjectStore.
package artifact

import "time"

// ArtifactType classifies the role of an artifact within a TestCase.
type ArtifactType string

const (
	// ArtifactTypeEnvironment is a kish detect environment snapshot.
	ArtifactTypeEnvironment ArtifactType = "environment"

	// ArtifactTypeResult is the primary test output or benchmark result.
	ArtifactTypeResult ArtifactType = "result"

	// ArtifactTypeScript is a test script associated with the run.
	ArtifactTypeScript ArtifactType = "script"

	// ArtifactTypeRaw is a raw collector output (e.g. package list files).
	ArtifactTypeRaw ArtifactType = "raw"

	// ArtifactTypeLog is a log file captured during a test run.
	ArtifactTypeLog ArtifactType = "log"

	// ArtifactTypeOther is used when no more specific type applies.
	ArtifactTypeOther ArtifactType = "other"
)

// validArtifactTypes is the set of accepted ArtifactType values.
var validArtifactTypes = map[ArtifactType]bool{
	ArtifactTypeEnvironment: true,
	ArtifactTypeResult:      true,
	ArtifactTypeScript:      true,
	ArtifactTypeRaw:         true,
	ArtifactTypeLog:         true,
	ArtifactTypeOther:       true,
}

// IsValidArtifactType reports whether t is an accepted artifact type.
func IsValidArtifactType(t ArtifactType) bool {
	return validArtifactTypes[t]
}

// Artifact is the metadata record for a file stored for a TestCase.
//
// File content is stored in an ObjectStore under StorageKey, not in MongoDB.
// The StorageKey must never be exposed in API responses.
type Artifact struct {
	// CaseID is the TestCase this artifact belongs to.
	CaseID string

	// ArtifactName is the logical filename, unique within a TestCase.
	// See ValidateArtifactName for the allowed character set and restrictions.
	ArtifactName string

	// ArtifactType classifies the artifact role.
	ArtifactType ArtifactType

	// ContentType is the MIME type of the stored file.
	ContentType string

	// Size is the byte length of the stored content.
	Size int64

	// SHA256 is the hex-encoded SHA-256 digest of the stored content.
	SHA256 string

	// StorageKey is the internal object store key (never exposed via API).
	// Format: "testcases/{case_id}/artifacts/{artifact_name}"
	StorageKey string

	// CreatedAt is when this artifact was first uploaded.
	CreatedAt time.Time

	// UpdatedAt is when this artifact was last replaced.
	UpdatedAt time.Time
}

// StorageKeyFor generates the canonical storage key for an artifact.
// The key format is: testcases/{caseID}/artifacts/{artifactName}
func StorageKeyFor(caseID, artifactName string) string {
	return "testcases/" + caseID + "/artifacts/" + artifactName
}
