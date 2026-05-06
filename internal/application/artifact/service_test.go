package artifact_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	appArtifact "github.com/AFDEAPAC/kish/internal/application/artifact"
	domArtifact "github.com/AFDEAPAC/kish/internal/domain/artifact"
	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
	"github.com/AFDEAPAC/kish/internal/infrastructure/storage"
)

// --- fake implementations ---

type fakeTCRepo struct {
	cases map[string]*testcase.TestCase
}

func newFakeTCRepo(ids ...string) *fakeTCRepo {
	r := &fakeTCRepo{cases: make(map[string]*testcase.TestCase)}
	for _, id := range ids {
		r.cases[id] = &testcase.TestCase{
			ID:       id,
			TestType: "generic",
			Environment: &environment.EnvironmentSnapshot{
				SchemaVersion: environment.SchemaVersionV1,
			},
			ResultArtifact: testcase.Artifact{Content: "dummy"},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}
	return r
}

func (r *fakeTCRepo) Create(_ context.Context, tc *testcase.TestCase) error {
	r.cases[tc.ID] = tc
	return nil
}
func (r *fakeTCRepo) Update(_ context.Context, id string, tc *testcase.TestCase) error {
	if _, ok := r.cases[id]; !ok {
		return testcase.ErrNotFound
	}
	r.cases[id] = tc
	return nil
}
func (r *fakeTCRepo) FindByID(_ context.Context, id string) (*testcase.TestCase, error) {
	tc, ok := r.cases[id]
	if !ok {
		return nil, testcase.ErrNotFound
	}
	return tc, nil
}

type fakeArtRepo struct {
	store map[string]*domArtifact.Artifact
}

func newFakeArtRepo() *fakeArtRepo {
	return &fakeArtRepo{store: make(map[string]*domArtifact.Artifact)}
}

func key(caseID, name string) string { return caseID + "/" + name }

func (r *fakeArtRepo) Upsert(_ context.Context, a *domArtifact.Artifact) error {
	existing, ok := r.store[key(a.CaseID, a.ArtifactName)]
	if ok {
		a.CreatedAt = existing.CreatedAt
	}
	r.store[key(a.CaseID, a.ArtifactName)] = a
	return nil
}
func (r *fakeArtRepo) FindOne(_ context.Context, caseID, name string) (*domArtifact.Artifact, error) {
	a, ok := r.store[key(caseID, name)]
	if !ok {
		return nil, domArtifact.ErrNotFound
	}
	return a, nil
}
func (r *fakeArtRepo) List(_ context.Context, caseID string) ([]*domArtifact.Artifact, error) {
	var result []*domArtifact.Artifact
	for k, v := range r.store {
		if strings.HasPrefix(k, caseID+"/") {
			result = append(result, v)
		}
	}
	return result, nil
}
func (r *fakeArtRepo) Delete(_ context.Context, caseID, name string) error {
	delete(r.store, key(caseID, name))
	return nil
}

type fakeObjectStore struct {
	objects map[string][]byte
}

func newFakeStore() *fakeObjectStore {
	return &fakeObjectStore{objects: make(map[string][]byte)}
}

func (s *fakeObjectStore) PutObject(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.objects[key] = data
	return nil
}
func (s *fakeObjectStore) GetObject(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, storage.ObjectInfo{}, storage.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), storage.ObjectInfo{Key: key, Size: int64(len(data))}, nil
}
func (s *fakeObjectStore) DeleteObject(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

// --- tests ---

func newSvc(tcIDs ...string) *appArtifact.Service {
	return appArtifact.NewService(newFakeTCRepo(tcIDs...), newFakeArtRepo(), newFakeStore())
}

func TestPutArtifact_Success(t *testing.T) {
	svc := newSvc("tc1")
	a, err := svc.PutArtifact(context.Background(), "tc1", "result.txt", "result", "text/plain", strings.NewReader("output"), -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.ArtifactName != "result.txt" {
		t.Errorf("expected name=result.txt, got %q", a.ArtifactName)
	}
	if a.SHA256 == "" {
		t.Error("expected SHA256 to be computed")
	}
	if a.Size != int64(len("output")) {
		t.Errorf("expected size=%d, got %d", len("output"), a.Size)
	}
}

func TestPutArtifact_TestCaseNotFound(t *testing.T) {
	svc := newSvc() // no pre-seeded cases
	_, err := svc.PutArtifact(context.Background(), "nonexistent", "result.txt", "result", "text/plain", strings.NewReader("data"), -1)
	if !errors.Is(err, testcase.ErrNotFound) {
		t.Errorf("expected testcase.ErrNotFound, got %v", err)
	}
}

func TestPutArtifact_InvalidName(t *testing.T) {
	svc := newSvc("tc1")
	_, err := svc.PutArtifact(context.Background(), "tc1", "../escape", "result", "text/plain", strings.NewReader("data"), -1)
	if err == nil {
		t.Error("expected validation error for invalid artifact name")
	}
}

func TestPutArtifact_InvalidType(t *testing.T) {
	svc := newSvc("tc1")
	_, err := svc.PutArtifact(context.Background(), "tc1", "file.txt", "badtype", "text/plain", strings.NewReader("data"), -1)
	if err == nil {
		t.Error("expected error for invalid artifact type")
	}
}

func TestPutArtifact_EmptyType_DefaultsToOther(t *testing.T) {
	svc := newSvc("tc1")
	a, err := svc.PutArtifact(context.Background(), "tc1", "file.txt", "", "text/plain", strings.NewReader("data"), -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.ArtifactType != domArtifact.ArtifactTypeOther {
		t.Errorf("expected type=other, got %q", a.ArtifactType)
	}
}

func TestPutArtifact_Overwrite_UpdatesMetadata(t *testing.T) {
	artRepo := newFakeArtRepo()
	svc := appArtifact.NewService(newFakeTCRepo("tc1"), artRepo, newFakeStore())

	_, _ = svc.PutArtifact(context.Background(), "tc1", "result.txt", "result", "text/plain", strings.NewReader("v1"), -1)
	a2, err := svc.PutArtifact(context.Background(), "tc1", "result.txt", "result", "text/plain", strings.NewReader("v2-longer"), -1)
	if err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	// Size must reflect v2 content.
	if a2.Size != int64(len("v2-longer")) {
		t.Errorf("expected size=%d after overwrite, got %d", len("v2-longer"), a2.Size)
	}
}

func TestGetArtifact_Success(t *testing.T) {
	svc := newSvc("tc1")
	_, _ = svc.PutArtifact(context.Background(), "tc1", "result.txt", "result", "text/plain", strings.NewReader("content"), -1)

	meta, rc, err := svc.GetArtifact(context.Background(), "tc1", "result.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()

	if meta.ArtifactName != "result.txt" {
		t.Errorf("unexpected name: %q", meta.ArtifactName)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != "content" {
		t.Errorf("unexpected content: %q", got)
	}
}

func TestGetArtifact_NotFound(t *testing.T) {
	svc := newSvc("tc1")
	_, _, err := svc.GetArtifact(context.Background(), "tc1", "missing.txt")
	if !errors.Is(err, domArtifact.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListArtifacts_ReturnsAll(t *testing.T) {
	svc := newSvc("tc1")
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		_, _ = svc.PutArtifact(context.Background(), "tc1", name, "other", "text/plain", strings.NewReader("x"), -1)
	}
	arts, err := svc.ListArtifacts(context.Background(), "tc1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(arts) != 3 {
		t.Errorf("expected 3 artifacts, got %d", len(arts))
	}
}

func TestDeleteArtifact_RemovesMetadataAndObject(t *testing.T) {
	svc := newSvc("tc1")
	_, _ = svc.PutArtifact(context.Background(), "tc1", "result.txt", "result", "text/plain", strings.NewReader("data"), -1)

	if err := svc.DeleteArtifact(context.Background(), "tc1", "result.txt"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, _, err := svc.GetArtifact(context.Background(), "tc1", "result.txt")
	if !errors.Is(err, domArtifact.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteArtifact_Missing_IsNil(t *testing.T) {
	svc := newSvc("tc1")
	if err := svc.DeleteArtifact(context.Background(), "tc1", "ghost.txt"); err != nil {
		t.Errorf("expected nil for missing artifact delete, got %v", err)
	}
}
