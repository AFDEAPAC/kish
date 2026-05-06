// Package testcase provides the application-layer service for managing TestCase entities.
//
// CreateMetadata creates a TestCase root without any inline artifact content.
// File content is uploaded separately through the artifact API after the TestCase
// is created, which is the canonical workflow as of the artifact API introduction.
package testcase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/testcase"
)

// Service coordinates TestCase creation and retrieval.
type Service struct {
	repo testcase.Repository
}

// NewService constructs a Service with the provided repository.
func NewService(repo testcase.Repository) *Service {
	return &Service{repo: repo}
}

// CreateMetadata generates a new TestCase ID, persists the metadata record, and
// returns the created TestCase. No artifact content is stored; callers must
// upload files via the artifact API after receiving the case_id.
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
		TestType:    testType,
		OwnerUserID: input.OwnerUserID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, tc); err != nil {
		return nil, fmt.Errorf("failed to persist testcase: %w", err)
	}
	return tc, nil
}

// GetTestCase retrieves a TestCase by ID.
func (s *Service) GetTestCase(ctx context.Context, id string) (*testcase.TestCase, error) {
	if id == "" {
		return nil, errors.New("testcase id is required")
	}
	return s.repo.FindByID(ctx, id)
}

// generateCaseID produces a unique case ID in the format TC-YYYYMMDDHHMMSS-xxxx.
func generateCaseID(t time.Time) (string, error) {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("TC-%s-%s", t.Format("20060102150405"), hex.EncodeToString(b)), nil
}
