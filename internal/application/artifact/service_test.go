package artifact_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	appArtifact "github.com/AFDEAPAC/kish/internal/application/artifact"
	domArtifact "github.com/AFDEAPAC/kish/internal/domain/artifact"
	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
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
func (r *fakeTCRepo) UpdatePartial(_ context.Context, id string, _ testcase.MetadataPatch) error {
	if _, ok := r.cases[id]; !ok {
		return testcase.ErrNotFound
	}
	return nil
}
func (r *fakeTCRepo) UpdateStatus(_ context.Context, id string, status testcase.Status, vis testcase.Visibility) error {
	tc, ok := r.cases[id]
	if !ok {
		return testcase.ErrNotFound
	}
	tc.Status = status
	tc.Visibility = vis
	return nil
}
func (r *fakeTCRepo) FindByID(_ context.Context, id string) (*testcase.TestCase, error) {
	tc, ok := r.cases[id]
	if !ok {
		return nil, testcase.ErrNotFound
	}
	return tc, nil
}
func (r *fakeTCRepo) List(_ context.Context, _ testcase.ListFilter) ([]*testcase.TestCase, error) {
	out := make([]*testcase.TestCase, 0, len(r.cases))
	for _, tc := range r.cases {
		out = append(out, tc)
	}
	return out, nil
}
func (r *fakeTCRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.cases[id]; !ok {
		return testcase.ErrNotFound
	}
	delete(r.cases, id)
	return nil
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
func (s *fakeObjectStore) GetObject(_ context.Context, key string) (io.ReadCloser, appArtifact.ObjectInfo, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, appArtifact.ObjectInfo{}, appArtifact.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), appArtifact.ObjectInfo{Key: key, Size: int64(len(data))}, nil
}
func (s *fakeObjectStore) DeleteObject(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

// --- tests ---

func newSvc(tcIDs ...string) *appArtifact.Service {
	return appArtifact.NewService(newFakeTCRepo(tcIDs...), newFakeArtRepo(), newFakeStore())
}

func validEnvJSON(scope string) string {
	return fmt.Sprintf(`{"schema_version":%q,"scope":%q,"type":"container","collected_at":"2026-05-06T13:24:15Z","data":{}}`, environment.SchemaVersionV1, scope)
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

func TestPutArtifact_LinksEnvironmentResultAndScriptToTestCase(t *testing.T) {
	tcRepo := newFakeTCRepo("tc1")
	svc := appArtifact.NewService(tcRepo, newFakeArtRepo(), newFakeStore())

	if _, err := svc.PutArtifact(context.Background(), "tc1", "env.json", "environment", "application/json", strings.NewReader(validEnvJSON("execution")), -1); err != nil {
		t.Fatalf("put env: %v", err)
	}
	tc := tcRepo.cases["tc1"]
	if len(tc.Environments) != 1 {
		t.Fatalf("expected 1 environment ref, got %d", len(tc.Environments))
	}
	if tc.Environments[0].Scope != environment.ScopeExecution {
		t.Errorf("expected execution scope, got %q", tc.Environments[0].Scope)
	}
	if tc.DefaultEnvironmentScope != environment.ScopeExecution {
		t.Errorf("expected default execution scope, got %q", tc.DefaultEnvironmentScope)
	}

	if _, err := svc.PutArtifact(context.Background(), "tc1", "result.txt", "result", "text/plain", strings.NewReader("output"), -1); err != nil {
		t.Fatalf("put result: %v", err)
	}
	if tcRepo.cases["tc1"].TestResult == nil || tcRepo.cases["tc1"].TestResult.ArtifactName != "result.txt" {
		t.Fatalf("expected testcase result ref to point at result.txt")
	}

	if _, err := svc.PutArtifact(context.Background(), "tc1", "run.sh", "script", "text/x-shellscript", strings.NewReader("#!/bin/sh"), -1); err != nil {
		t.Fatalf("put script: %v", err)
	}
	if len(tcRepo.cases["tc1"].TestScripts) != 1 || tcRepo.cases["tc1"].TestScripts[0].ArtifactName != "run.sh" {
		t.Fatalf("expected testcase script ref to point at run.sh")
	}
}

func TestPutArtifact_ReplacesEnvironmentByScope(t *testing.T) {
	tcRepo := newFakeTCRepo("tc1")
	svc := appArtifact.NewService(tcRepo, newFakeArtRepo(), newFakeStore())

	_, _ = svc.PutArtifact(context.Background(), "tc1", "env-v1.json", "environment", "application/json", strings.NewReader(validEnvJSON("execution")), -1)
	if _, err := svc.PutArtifact(context.Background(), "tc1", "env-v2.json", "environment", "application/json", strings.NewReader(validEnvJSON("execution")), -1); err != nil {
		t.Fatalf("replace env: %v", err)
	}
	envs := tcRepo.cases["tc1"].Environments
	if len(envs) != 1 {
		t.Fatalf("expected replacement to keep 1 execution ref, got %d", len(envs))
	}
	if envs[0].ArtifactName != "env-v2.json" {
		t.Errorf("expected env-v2.json, got %q", envs[0].ArtifactName)
	}
}

func TestPutArtifact_AddsSupportingEnvironmentWithoutChangingDefault(t *testing.T) {
	tcRepo := newFakeTCRepo("tc1")
	svc := appArtifact.NewService(tcRepo, newFakeArtRepo(), newFakeStore())

	_, _ = svc.PutArtifact(context.Background(), "tc1", "env.json", "environment", "application/json", strings.NewReader(validEnvJSON("execution")), -1)
	if _, err := svc.PutArtifact(context.Background(), "tc1", "host.json", "environment", "application/json", strings.NewReader(validEnvJSON("supporting")), -1); err != nil {
		t.Fatalf("put supporting env: %v", err)
	}
	tc := tcRepo.cases["tc1"]
	if len(tc.Environments) != 2 {
		t.Fatalf("expected execution + supporting refs, got %d", len(tc.Environments))
	}
	if tc.DefaultEnvironmentScope != environment.ScopeExecution {
		t.Errorf("expected default to remain execution, got %q", tc.DefaultEnvironmentScope)
	}
}

func TestPutArtifact_InvalidEnvironmentSnapshot(t *testing.T) {
	svc := newSvc("tc1")
	_, err := svc.PutArtifact(context.Background(), "tc1", "env.json", "environment", "application/json", strings.NewReader(`{"schema_version":"bad"}`), -1)
	if !errors.Is(err, environment.ErrInvalidSnapshot) {
		t.Fatalf("expected ErrInvalidSnapshot, got %v", err)
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

// Published TestCases must reject result and environment writes/deletes;
// other artifact types remain mutable so scripts and snapshots can be appended.
func TestPutArtifact_PublishedResultImmutable(t *testing.T) {
	tcRepo := newFakeTCRepo("tc1")
	tcRepo.cases["tc1"].Status = testcase.StatusPublished
	tcRepo.cases["tc1"].Visibility = testcase.VisibilityPublic
	svc := appArtifact.NewService(tcRepo, newFakeArtRepo(), newFakeStore())

	_, err := svc.PutArtifact(context.Background(), "tc1", "result.txt", "result", "text/plain", strings.NewReader("v2"), -1)
	if !errors.Is(err, testcase.ErrPublishedImmutable) {
		t.Errorf("expected ErrPublishedImmutable for result on published, got %v", err)
	}

	_, err = svc.PutArtifact(context.Background(), "tc1", "env.json", "environment", "application/json", strings.NewReader(validEnvJSON("execution")), -1)
	if !errors.Is(err, testcase.ErrPublishedImmutable) {
		t.Errorf("expected ErrPublishedImmutable for environment on published, got %v", err)
	}
}

func TestPutArtifact_PublishedSupportingEnvironmentAllowed(t *testing.T) {
	tcRepo := newFakeTCRepo("tc1")
	tcRepo.cases["tc1"].Status = testcase.StatusPublished
	tcRepo.cases["tc1"].Visibility = testcase.VisibilityPublic
	svc := appArtifact.NewService(tcRepo, newFakeArtRepo(), newFakeStore())

	_, err := svc.PutArtifact(context.Background(), "tc1", "host.json", "environment", "application/json", strings.NewReader(validEnvJSON("supporting")), -1)
	if err != nil {
		t.Errorf("expected supporting environment upload to succeed on published testcase, got %v", err)
	}
}

func TestPutArtifact_PublishedScriptAllowed(t *testing.T) {
	tcRepo := newFakeTCRepo("tc1")
	tcRepo.cases["tc1"].Status = testcase.StatusPublished
	tcRepo.cases["tc1"].Visibility = testcase.VisibilityPublic
	svc := appArtifact.NewService(tcRepo, newFakeArtRepo(), newFakeStore())

	_, err := svc.PutArtifact(context.Background(), "tc1", "extra.sh", "script", "text/x-shellscript", strings.NewReader("#!/bin/sh"), -1)
	if err != nil {
		t.Errorf("expected script append to succeed on published, got %v", err)
	}
}

func TestDeleteArtifact_PublishedResultImmutable(t *testing.T) {
	tcRepo := newFakeTCRepo("tc1")
	svc := appArtifact.NewService(tcRepo, newFakeArtRepo(), newFakeStore())
	_, _ = svc.PutArtifact(context.Background(), "tc1", "result.txt", "result", "text/plain", strings.NewReader("v1"), -1)

	tcRepo.cases["tc1"].Status = testcase.StatusPublished
	tcRepo.cases["tc1"].Visibility = testcase.VisibilityPublic

	if err := svc.DeleteArtifact(context.Background(), "tc1", "result.txt"); !errors.Is(err, testcase.ErrPublishedImmutable) {
		t.Errorf("expected ErrPublishedImmutable, got %v", err)
	}
}
