// Package artifact provides the application-layer service for managing TestCase artifacts.
//
// The service coordinates artifact name validation, SHA-256 computation, TestCase existence
// checks, ObjectStore writes, and metadata persistence. All workflow rules live here;
// HTTP handlers and repository implementations must remain free of business logic.
package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	domartifact "github.com/AFDEAPAC/kish/internal/domain/artifact"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
	"github.com/AFDEAPAC/kish/internal/infrastructure/storage"
)

// Service coordinates artifact upload, retrieval, listing, and deletion.
type Service struct {
	tcRepo  testcase.Repository
	artRepo domartifact.Repository
	store   storage.ObjectStore
}

// NewService constructs a Service with the provided dependencies.
func NewService(tcRepo testcase.Repository, artRepo domartifact.Repository, store storage.ObjectStore) *Service {
	return &Service{tcRepo: tcRepo, artRepo: artRepo, store: store}
}

// PutArtifact uploads artifact content and upserts its metadata record.
//
// Workflow:
//  1. Validate artifactName and artifactType.
//  2. Verify the TestCase exists (returns testcase.ErrNotFound if not).
//  3. Stream r through a SHA-256 hasher while writing to the ObjectStore.
//  4. Upsert the artifact metadata in the repository.
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

	// Verify TestCase exists before writing any content.
	if _, err := s.tcRepo.FindByID(ctx, caseID); err != nil {
		return nil, err
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
		if errors.Is(err, storage.ErrObjectNotFound) {
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

	// Delete object first; if it's already gone, continue to clean up metadata.
	if err := s.store.DeleteObject(ctx, meta.StorageKey); err != nil && !errors.Is(err, storage.ErrObjectNotFound) {
		return fmt.Errorf("delete artifact content: %w", err)
	}

	return s.artRepo.Delete(ctx, caseID, artifactName)
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
