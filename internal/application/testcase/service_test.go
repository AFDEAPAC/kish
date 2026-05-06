package testcase_test

import (
	"context"
	"strings"
	"testing"

	apptestcase "github.com/AFDEAPAC/kish/internal/application/testcase"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
)

// fakeRepo is an in-memory implementation of testcase.Repository for testing.
type fakeRepo struct {
	store map[string]*testcase.TestCase
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{store: make(map[string]*testcase.TestCase)}
}

func (r *fakeRepo) Create(_ context.Context, tc *testcase.TestCase) error {
	r.store[tc.ID] = tc
	return nil
}

// Update is required by the testcase.Repository interface. It is retained here
// even though the service no longer calls it, to satisfy the interface contract.
func (r *fakeRepo) Update(_ context.Context, id string, tc *testcase.TestCase) error {
	if _, ok := r.store[id]; !ok {
		return testcase.ErrNotFound
	}
	r.store[id] = tc
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id string) (*testcase.TestCase, error) {
	tc, ok := r.store[id]
	if !ok {
		return nil, testcase.ErrNotFound
	}
	return tc, nil
}

func TestCreateMetadata_GeneratesID(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, err := svc.CreateMetadata(context.Background(), testcase.MetadataInput{
		Name:     "my test",
		TestType: "generic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.ID == "" {
		t.Error("expected non-empty ID")
	}
	if !strings.HasPrefix(tc.ID, "TC-") {
		t.Errorf("expected ID to start with TC-, got %q", tc.ID)
	}
}

func TestCreateMetadata_DefaultsTestType(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, err := svc.CreateMetadata(context.Background(), testcase.MetadataInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.TestType != "generic" {
		t.Errorf("expected test_type=generic, got %q", tc.TestType)
	}
}

func TestCreateMetadata_PreservesName(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, err := svc.CreateMetadata(context.Background(), testcase.MetadataInput{
		Name:     "sglang benchmark",
		TestType: "sglang-benchmark",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.Name != "sglang benchmark" {
		t.Errorf("expected name=%q, got %q", "sglang benchmark", tc.Name)
	}
	if tc.TestType != "sglang-benchmark" {
		t.Errorf("expected test_type=sglang-benchmark, got %q", tc.TestType)
	}
}

func TestCreateMetadata_SetsTimestamps(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, err := svc.CreateMetadata(context.Background(), testcase.MetadataInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
	if !tc.UpdatedAt.Equal(tc.CreatedAt) {
		t.Error("expected created_at == updated_at on creation")
	}
}

func TestCreateMetadata_NoInlineArtifacts(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, err := svc.CreateMetadata(context.Background(), testcase.MetadataInput{
		TestType: "generic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Metadata-only creation must not populate inline artifact fields.
	if tc.Environment != nil {
		t.Error("expected Environment to be nil for metadata-only create")
	}
	if tc.ResultArtifact.Content != "" {
		t.Error("expected empty ResultArtifact for metadata-only create")
	}
	if len(tc.ScriptArtifacts) != 0 {
		t.Errorf("expected 0 ScriptArtifacts, got %d", len(tc.ScriptArtifacts))
	}
}

func TestGetTestCase_ReturnsCreatedCase(t *testing.T) {
	repo := newFakeRepo()
	svc := apptestcase.NewService(repo)

	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{
		TestType: "generic",
	})

	got, err := svc.GetTestCase(context.Background(), tc.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != tc.ID {
		t.Errorf("expected id=%q, got %q", tc.ID, got.ID)
	}
}

func TestGetTestCase_NotFound(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	_, err := svc.GetTestCase(context.Background(), "TC-nonexistent")
	if err == nil {
		t.Error("expected error for non-existent ID")
	}
}
