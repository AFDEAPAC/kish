// Package testcase provides the application-layer service for managing TestCase entities.
//
// The service is responsible for enforcing all TestCase domain invariants:
//
//   - New TestCases are always created as Draft + Private.
//   - test_type can be edited only while a TestCase is in the Draft state.
//   - Visibility can be changed only while a TestCase is Published.
//   - Only Draft TestCases can transition to Published; the publisher chooses
//     visibility (default public) at the same time.
//   - Ownership: developers may modify only their own TestCases; admins may
//     modify all TestCases. Ownership is checked here so handlers stay thin.
//
// File content (results, environment snapshots, scripts) is stored separately
// through the artifact API; this service deals only with metadata.
package testcase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	domartifact "github.com/AFDEAPAC/kish/internal/domain/artifact"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
)

// Service coordinates TestCase metadata creation, retrieval, listing, update,
// and the publish workflow.
type Service struct {
	repo    testcase.Repository
	cleanup DeleteCleanup
}

// DeleteCleanup carries optional dependencies used to remove artifact content
// and metadata as part of deleting a TestCase.
type DeleteCleanup struct {
	ArtifactRepo artifactRepository
	Store        objectStore
}

type artifactRepository interface {
	List(ctx context.Context, caseID string) ([]*domartifact.Artifact, error)
	Delete(ctx context.Context, caseID, artifactName string) error
}

type objectStore interface {
	DeleteObject(ctx context.Context, key string) error
}

// NewService constructs a Service with the provided repository.
func NewService(repo testcase.Repository, cleanup ...DeleteCleanup) *Service {
	var c DeleteCleanup
	if len(cleanup) > 0 {
		c = cleanup[0]
	}
	return &Service{repo: repo, cleanup: c}
}

// CreateMetadata generates a new TestCase ID, persists the metadata record, and
// returns the created TestCase. No artifact content is stored; callers must
// upload files via the artifact API after receiving the case_id.
//
// New TestCases are always created as Draft+Private.
func (s *Service) CreateMetadata(ctx context.Context, input testcase.MetadataInput) (*testcase.TestCase, error) {
	testType := input.TestType
	if testType == "" {
		testType = "generic"
	}

	now := time.Now().UTC()
	id, err := generateCaseID(now)
	if err != nil {
		return nil, fmt.Errorf("failed to generate case id: %w", err)
	}

	tc := &testcase.TestCase{
		ID:          id,
		Name:        input.Name,
		Description: input.Description,
		TestType:    testType,
		Tags:        normaliseTags(input.Tags),
		Status:      testcase.StatusDraft,
		Visibility:  testcase.VisibilityPrivate,
		OwnerUserID: input.OwnerUserID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, tc); err != nil {
		return nil, fmt.Errorf("failed to persist testcase: %w", err)
	}
	return tc, nil
}

// GetTestCase retrieves a TestCase by ID without applying any visibility
// filtering. Callers are responsible for authorising the read.
func (s *Service) GetTestCase(ctx context.Context, id string) (*testcase.TestCase, error) {
	if id == "" {
		return nil, errors.New("testcase id is required")
	}
	return s.repo.FindByID(ctx, id)
}

// ListTestCases returns TestCases visible to the caller described by filter.
// All authorization filtering happens at the repository layer based on
// filter.Selector + caller identity.
func (s *Service) ListTestCases(ctx context.Context, filter testcase.ListFilter) ([]*testcase.TestCase, error) {
	return s.repo.List(ctx, filter)
}

// UpdateMetadata applies a partial metadata patch with full invariant enforcement.
//
// The caller must provide callerUserID/callerIsAdmin so ownership can be checked.
// Returns:
//   - testcase.ErrNotFound if the TestCase does not exist.
//   - testcase.ErrForbidden if the caller is not the owner (and is not admin).
//   - testcase.ErrNotDraft if the patch tries to change test_type on a non-draft.
//   - testcase.ErrNotPublished if the patch tries to change visibility on a non-published.
//   - a validation error wrapped with fmt.Errorf for invalid field values.
func (s *Service) UpdateMetadata(
	ctx context.Context,
	id string,
	patch testcase.MetadataPatch,
	callerUserID string,
	callerIsAdmin bool,
) (*testcase.TestCase, error) {
	if id == "" {
		return nil, errors.New("testcase id is required")
	}

	tc, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canManage(tc, callerUserID, callerIsAdmin) {
		return nil, testcase.ErrForbidden
	}

	if patch.TestType != nil {
		if !tc.IsDraft() {
			return nil, testcase.ErrNotDraft
		}
		if *patch.TestType == "" {
			return nil, fmt.Errorf("test_type cannot be empty")
		}
	}
	if patch.Visibility != nil {
		if !tc.IsPublished() {
			return nil, testcase.ErrNotPublished
		}
		if !patch.Visibility.IsValid() {
			return nil, fmt.Errorf("invalid visibility %q", *patch.Visibility)
		}
	}
	if patch.Tags != nil {
		normalised := normaliseTags(*patch.Tags)
		patch.Tags = &normalised
	}

	if err := s.repo.UpdatePartial(ctx, id, patch); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// Publish transitions a Draft TestCase to Published with the given visibility.
// Visibility defaults to VisibilityPublic when the empty string is supplied.
//
// Returns testcase.ErrNotDraft when called on an already-published TestCase.
func (s *Service) Publish(
	ctx context.Context,
	id string,
	visibility testcase.Visibility,
	callerUserID string,
	callerIsAdmin bool,
) (*testcase.TestCase, error) {
	if id == "" {
		return nil, errors.New("testcase id is required")
	}
	if visibility == "" {
		visibility = testcase.VisibilityPublic
	}
	if !visibility.IsValid() {
		return nil, fmt.Errorf("invalid visibility %q", visibility)
	}

	tc, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canManage(tc, callerUserID, callerIsAdmin) {
		return nil, testcase.ErrForbidden
	}
	if !tc.IsDraft() {
		return nil, testcase.ErrNotDraft
	}

	if err := s.repo.UpdateStatus(ctx, id, testcase.StatusPublished, visibility); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// DeleteTestCase removes a TestCase and its artifact records/content.
// Ownership is checked before any destructive work is performed.
func (s *Service) DeleteTestCase(ctx context.Context, id, callerUserID string, callerIsAdmin bool) error {
	if id == "" {
		return errors.New("testcase id is required")
	}
	tc, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !canManage(tc, callerUserID, callerIsAdmin) {
		return testcase.ErrForbidden
	}

	if s.cleanup.ArtifactRepo != nil {
		artifacts, err := s.cleanup.ArtifactRepo.List(ctx, id)
		if err != nil {
			return fmt.Errorf("list testcase artifacts for delete: %w", err)
		}
		for _, a := range artifacts {
			if s.cleanup.Store != nil && a.StorageKey != "" {
				if err := s.cleanup.Store.DeleteObject(ctx, a.StorageKey); err != nil {
					return fmt.Errorf("delete artifact content %q: %w", a.ArtifactName, err)
				}
			}
			if err := s.cleanup.ArtifactRepo.Delete(ctx, id, a.ArtifactName); err != nil {
				return fmt.Errorf("delete artifact metadata %q: %w", a.ArtifactName, err)
			}
		}
	}

	return s.repo.Delete(ctx, id)
}

// CanRead reports whether the caller may read the given TestCase, based on
// status, visibility, ownership, and admin role. Used by handlers to enforce
// visibility on GET/list endpoints.
func (s *Service) CanRead(tc *testcase.TestCase, callerUserID string, callerIsAdmin bool) bool {
	if tc == nil {
		return false
	}
	if callerIsAdmin {
		return true
	}
	if tc.IsPubliclyReadable() {
		return true
	}
	// Non-public: only the owner may read. Anonymous and other developers cannot.
	if callerUserID == "" || tc.OwnerUserID == "" {
		return false
	}
	return tc.OwnerUserID == callerUserID
}

// canManage reports whether the caller may mutate the TestCase.
// Only admins or the explicit owner may manage. Legacy TestCases without an
// owner are admin-only.
func canManage(tc *testcase.TestCase, callerUserID string, callerIsAdmin bool) bool {
	if callerIsAdmin {
		return true
	}
	if tc.OwnerUserID == "" || callerUserID == "" {
		return false
	}
	return tc.OwnerUserID == callerUserID
}

// normaliseTags trims whitespace and drops empty entries while preserving order.
// A nil input becomes a non-nil empty slice so persistence treats "no tags" and
// "field absent" identically.
func normaliseTags(in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		trimmed := trimSpace(t)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

// trimSpace is a tiny wrapper to avoid importing strings just for this helper.
func trimSpace(s string) string {
	for len(s) > 0 && isASCIISpace(s[0]) {
		s = s[1:]
	}
	for len(s) > 0 && isASCIISpace(s[len(s)-1]) {
		s = s[:len(s)-1]
	}
	return s
}

func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n':
		return true
	}
	return false
}

// generateCaseID produces a unique case ID in the format TC-YYYYMMDDHHMMSS-xxxx.
func generateCaseID(t time.Time) (string, error) {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("TC-%s-%s", t.Format("20060102150405"), hex.EncodeToString(b)), nil
}
