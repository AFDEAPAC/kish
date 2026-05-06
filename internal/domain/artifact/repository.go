package artifact

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// ErrNotFound is returned when the requested artifact does not exist.
var ErrNotFound = errors.New("artifact not found")

// Repository is the persistence interface for Artifact metadata.
//
// Implementations live in the infrastructure layer (e.g. infrastructure/mongodb).
// The domain and application layers depend only on this interface.
type Repository interface {
	// Upsert creates or replaces the artifact metadata record identified by
	// CaseID + ArtifactName. On insert, CreatedAt is set; on update, it is preserved.
	Upsert(ctx context.Context, a *Artifact) error

	// FindOne retrieves artifact metadata by case ID and artifact name.
	// Returns ErrNotFound if no matching record exists.
	FindOne(ctx context.Context, caseID, artifactName string) (*Artifact, error)

	// List returns all artifact metadata records for the given case ID,
	// ordered by artifact name. Returns an empty slice (not error) when none exist.
	List(ctx context.Context, caseID string) ([]*Artifact, error)

	// Delete removes the artifact metadata record for the given case ID and name.
	// If the record does not exist, nil is returned (idempotent).
	Delete(ctx context.Context, caseID, artifactName string) error
}

// artifactNamePattern restricts artifact names to safe filename characters.
// Allowed: a-z, A-Z, 0-9, dot, underscore, hyphen.
var artifactNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ValidateArtifactName checks that name is safe to use as an artifact identifier.
//
// Rules:
//   - non-empty
//   - length 1–255 characters
//   - must not be "." or ".."
//   - must not contain "/" or "\" or null byte
//   - must match [a-zA-Z0-9._-]
//
// This validation is intentionally strict to prevent path traversal and to keep
// artifact names portable across operating systems and storage backends.
func ValidateArtifactName(name string) error {
	if name == "" {
		return errors.New("artifact_name must not be empty")
	}
	if len(name) > 255 {
		return fmt.Errorf("artifact_name must not exceed 255 characters (got %d)", len(name))
	}
	if name == "." || name == ".." {
		return fmt.Errorf("artifact_name %q is not allowed", name)
	}
	// Guard against null bytes even though the regexp below would also catch them.
	for i := 0; i < len(name); i++ {
		if name[i] == 0 {
			return errors.New("artifact_name must not contain null bytes")
		}
	}
	if !artifactNamePattern.MatchString(name) {
		return fmt.Errorf("artifact_name %q contains invalid characters; allowed: a-z A-Z 0-9 . _ -", name)
	}
	return nil
}
