// Package artifact provides the application-layer service for managing TestCase artifacts.
//
// The service coordinates artifact name validation, SHA-256 computation, TestCase existence
// checks, ObjectStore writes, and metadata persistence. All workflow rules live here;
// HTTP handlers and repository implementations must remain free of business logic.
package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	domartifact "github.com/AFDEAPAC/kish/internal/domain/artifact"
	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
)

// Service coordinates artifact upload, retrieval, listing, and deletion.
type Service struct {
	tcRepo  testcase.Repository
	artRepo domartifact.Repository
	store   ObjectStore
}

// NewService constructs a Service with the provided dependencies.
func NewService(tcRepo testcase.Repository, artRepo domartifact.Repository, store ObjectStore) *Service {
	return &Service{tcRepo: tcRepo, artRepo: artRepo, store: store}
}

// PutArtifact uploads artifact content and upserts its metadata record.
//
// Workflow:
//  1. Validate artifactName and artifactType.
//  2. Verify the TestCase exists (returns testcase.ErrNotFound if not).
//  3. Parse environment snapshots before writing so scope-specific publish
//     immutability can be enforced.
//  4. Stream r through a SHA-256 hasher while writing to the ObjectStore.
//  5. Upsert the artifact metadata in the repository.
//  6. Link canonical artifact types back to the TestCase aggregate state.
func (s *Service) PutArtifact(
	ctx context.Context,
	caseID, artifactName, artifactType, contentType string,
	r io.Reader,
	size int64,
) (*domartifact.Artifact, error) {
	if err := domartifact.ValidateArtifactName(artifactName); err != nil {
		return nil, err
	}

	artType := domartifact.ArtifactType(artifactType)
	if artifactType == "" {
		artType = domartifact.ArtifactTypeOther
	} else if !domartifact.IsValidArtifactType(artType) {
		return nil, fmt.Errorf("invalid artifact_type %q; allowed: environment result script raw log other", artifactType)
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Verify TestCase exists and is mutable for this artifact type.
	tc, err := s.tcRepo.FindByID(ctx, caseID)
	if err != nil {
		return nil, err
	}

	var envSnap *environment.EnvironmentSnapshot
	if artType == domartifact.ArtifactTypeEnvironment {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("read environment artifact: %w", err)
		}
		envSnap, err = environment.ParseSnapshot(data)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}

	if tc.IsPublished() && !canWritePublishedArtifact(artType, envSnap) {
		return nil, testcase.ErrPublishedImmutable
	}

	storageKey := domartifact.StorageKeyFor(caseID, artifactName)

	// Compute SHA-256 inline while streaming to the ObjectStore to avoid
	// loading the full content into memory.
	hasher := sha256.New()
	counter := &countingReader{r: io.TeeReader(r, hasher)}

	if err := s.store.PutObject(ctx, storageKey, counter, size, contentType); err != nil {
		return nil, fmt.Errorf("store artifact %q: %w", artifactName, err)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	now := time.Now().UTC()

	a := &domartifact.Artifact{
		CaseID:       caseID,
		ArtifactName: artifactName,
		ArtifactType: artType,
		ContentType:  contentType,
		Size:         counter.n,
		SHA256:       checksum,
		StorageKey:   storageKey,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.artRepo.Upsert(ctx, a); err != nil {
		return nil, fmt.Errorf("save artifact metadata: %w", err)
	}

	if err := s.linkArtifactToTestCase(ctx, tc, a, envSnap); err != nil {
		return nil, err
	}
	return a, nil
}

// GetArtifact retrieves artifact metadata and opens a stream to its content.
// The caller must close the returned ReadCloser.
// Returns artifact.ErrNotFound if the artifact does not exist.
func (s *Service) GetArtifact(ctx context.Context, caseID, artifactName string) (*domartifact.Artifact, io.ReadCloser, error) {
	if err := domartifact.ValidateArtifactName(artifactName); err != nil {
		return nil, nil, err
	}

	meta, err := s.artRepo.FindOne(ctx, caseID, artifactName)
	if err != nil {
		return nil, nil, err
	}

	rc, _, err := s.store.GetObject(ctx, meta.StorageKey)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			// Metadata exists but content is gone (e.g. manual deletion from disk).
			// Treat as not found from the API perspective.
			return nil, nil, domartifact.ErrNotFound
		}
		return nil, nil, fmt.Errorf("read artifact content: %w", err)
	}
	return meta, rc, nil
}

// ListArtifacts returns all artifact metadata records for the given TestCase.
func (s *Service) ListArtifacts(ctx context.Context, caseID string) ([]*domartifact.Artifact, error) {
	return s.artRepo.List(ctx, caseID)
}

// DeleteArtifact removes both the stored object and its metadata record.
// If the metadata does not exist, nil is returned (idempotent).
// If the metadata exists but the object is already gone from the store,
// the metadata is still removed so the system stays consistent.
//
// Deletion of result and environment artifacts is rejected once the parent
// TestCase has been published, mirroring the upload immutability rule. Draft
// result deletes also remove the corresponding TestCase result reference so
// readers do not see stale canonical state.
func (s *Service) DeleteArtifact(ctx context.Context, caseID, artifactName string) error {
	if err := domartifact.ValidateArtifactName(artifactName); err != nil {
		return err
	}

	meta, err := s.artRepo.FindOne(ctx, caseID, artifactName)
	if err != nil {
		if errors.Is(err, domartifact.ErrNotFound) {
			return nil
		}
		return err
	}

	var tc *testcase.TestCase
	if isImmutableOnPublished(meta.ArtifactType) {
		tc, err = s.tcRepo.FindByID(ctx, caseID)
		if err != nil && !errors.Is(err, testcase.ErrNotFound) {
			return err
		}
		if tc != nil && tc.IsPublished() {
			return testcase.ErrPublishedImmutable
		}
	}

	// Delete object first; if it's already gone, continue to clean up metadata.
	if err := s.store.DeleteObject(ctx, meta.StorageKey); err != nil && !errors.Is(err, ErrObjectNotFound) {
		return fmt.Errorf("delete artifact content: %w", err)
	}

	if err := s.artRepo.Delete(ctx, caseID, artifactName); err != nil {
		return err
	}
	return s.unlinkDeletedArtifactFromTestCase(ctx, tc, meta)
}

// canWritePublishedArtifact reports whether an artifact upload may mutate a
// published TestCase. Result and execution environment artifacts are canonical
// facts and remain frozen; supporting environments and scripts may be appended.
//
// Environment artifacts must be parsed before this check so supporting
// snapshots can be distinguished from the execution environment.
func canWritePublishedArtifact(t domartifact.ArtifactType, envSnap *environment.EnvironmentSnapshot) bool {
	switch t {
	case domartifact.ArtifactTypeResult:
		return false
	case domartifact.ArtifactTypeEnvironment:
		return envSnap != nil && envSnap.Scope == environment.ScopeSupporting
	}
	return true
}

// isImmutableOnPublished reports whether deleting a stored artifact is blocked
// after publication. Deletes operate on existing metadata and do not include the
// uploaded environment payload needed to distinguish execution from supporting,
// so environment deletes remain conservative.
func isImmutableOnPublished(t domartifact.ArtifactType) bool {
	switch t {
	case domartifact.ArtifactTypeResult, domartifact.ArtifactTypeEnvironment:
		return true
	}
	return false
}

// unlinkDeletedArtifactFromTestCase removes the deleted artifact's reference
// from the parent TestCase aggregate.
//
// Only result artifacts carry a canonical TestCase-level reference; for other
// artifact types this is a no-op. The caller-supplied tc is the TestCase that
// was already fetched for the publish-immutability check (when applicable);
// when nil, this helper re-fetches by CaseID. A missing TestCase is treated
// as "already gone" — callers should not fail an otherwise-successful delete
// because the parent vanished between checks. Update is only issued when an
// entry actually changed, to avoid spurious writes when re-deleting a result
// whose link was previously cleaned up.
func (s *Service) unlinkDeletedArtifactFromTestCase(ctx context.Context, tc *testcase.TestCase, meta *domartifact.Artifact) error {
	if meta.ArtifactType != domartifact.ArtifactTypeResult {
		return nil
	}
	if tc == nil {
		var err error
		tc, err = s.tcRepo.FindByID(ctx, meta.CaseID)
		if err != nil {
			if errors.Is(err, testcase.ErrNotFound) {
				return nil
			}
			return err
		}
	}
	if !tc.RemoveTestResultArtifact(meta.ArtifactName) {
		return nil
	}
	tc.UpdatedAt = time.Now().UTC()
	if err := s.tcRepo.Update(ctx, tc.ID, tc); err != nil {
		return fmt.Errorf("unlink deleted result from testcase: %w", err)
	}
	return nil
}

// FindTestCase returns the parent TestCase for visibility checks performed by
// HTTP handlers (e.g. anonymous artifact reads must verify the parent is
// public-published). Returns testcase.ErrNotFound when the case does not exist.
func (s *Service) FindTestCase(ctx context.Context, caseID string) (*testcase.TestCase, error) {
	return s.tcRepo.FindByID(ctx, caseID)
}

// CheckOwnership verifies that the TestCase identified by caseID is owned by ownerUserID.
// Returns testcase.ErrNotFound if the TestCase does not exist.
// Returns a non-nil error if the TestCase exists but belongs to a different user.
// TestCases with an empty OwnerUserID (legacy, pre-ownership) are treated as
// admin-only; this method returns an ownership error for non-owners.
func (s *Service) CheckOwnership(ctx context.Context, caseID, ownerUserID string) error {
	tc, err := s.tcRepo.FindByID(ctx, caseID)
	if err != nil {
		return err
	}
	if tc.OwnerUserID != ownerUserID {
		return fmt.Errorf("testcase is not owned by user %q", ownerUserID)
	}
	return nil
}

// linkArtifactToTestCase records the just-uploaded artifact in the parent
// TestCase aggregate so reads see canonical metadata without scanning the
// artifact collection.
//
// Publish-time immutability is already enforced by PutArtifact before this
// helper runs; here we may safely upsert. Each canonical type maps to its
// own slice on TestCase and is keyed by ArtifactName, so re-uploading the
// same name refreshes metadata in place. Non-canonical types (raw/log/other)
// are intentionally left out of the aggregate — they are listed via the
// artifact API and do not need a TestCase-level summary.
func (s *Service) linkArtifactToTestCase(
	ctx context.Context,
	tc *testcase.TestCase,
	a *domartifact.Artifact,
	envSnap *environment.EnvironmentSnapshot,
) error {
	switch a.ArtifactType {
	case domartifact.ArtifactTypeEnvironment:
		if envSnap == nil {
			return fmt.Errorf("%w: missing parsed environment snapshot", environment.ErrInvalidSnapshot)
		}
		tc.SetEnvironmentArtifact(testcase.EnvironmentArtifactRef{
			Scope:           envSnap.Scope,
			ArtifactName:    a.ArtifactName,
			SchemaVersion:   envSnap.SchemaVersion,
			EnvironmentType: envSnap.Type,
			CollectedAt:     envSnap.CollectedAt,
			ContentType:     a.ContentType,
			Size:            a.Size,
			SHA256:          a.SHA256,
			UploadedAt:      a.UpdatedAt,
		})
	case domartifact.ArtifactTypeResult:
		tc.UpsertTestResultArtifact(testcase.TestResultArtifactRef{
			ArtifactName: a.ArtifactName,
			ContentType:  a.ContentType,
			Size:         a.Size,
			SHA256:       a.SHA256,
			UploadedAt:   a.UpdatedAt,
		})
	case domartifact.ArtifactTypeScript:
		tc.UpsertTestScriptArtifact(testcase.TestScriptArtifactRef{
			ArtifactName: a.ArtifactName,
			ContentType:  a.ContentType,
			Size:         a.Size,
			SHA256:       a.SHA256,
			UploadedAt:   a.UpdatedAt,
		})
	default:
		return nil
	}

	tc.UpdatedAt = time.Now().UTC()
	if err := s.tcRepo.Update(ctx, tc.ID, tc); err != nil {
		return fmt.Errorf("link artifact to testcase: %w", err)
	}
	return nil
}

// countingReader wraps an io.Reader and counts total bytes read.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
