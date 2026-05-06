package testcase_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	apptestcase "github.com/AFDEAPAC/kish/internal/application/testcase"
	domartifact "github.com/AFDEAPAC/kish/internal/domain/artifact"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
)

// fakeRepo is an in-memory implementation of testcase.Repository for testing.
//
// It satisfies the full Repository interface including UpdatePartial,
// UpdateStatus, and List so the service can be exercised end-to-end.
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
// even though the service no longer calls it directly, to satisfy the contract.
func (r *fakeRepo) Update(_ context.Context, id string, tc *testcase.TestCase) error {
	if _, ok := r.store[id]; !ok {
		return testcase.ErrNotFound
	}
	r.store[id] = tc
	return nil
}

func (r *fakeRepo) UpdatePartial(_ context.Context, id string, patch testcase.MetadataPatch) error {
	tc, ok := r.store[id]
	if !ok {
		return testcase.ErrNotFound
	}
	if patch.Name != nil {
		tc.Name = *patch.Name
	}
	if patch.Description != nil {
		tc.Description = *patch.Description
	}
	if patch.TestType != nil {
		tc.TestType = *patch.TestType
	}
	if patch.Tags != nil {
		tc.Tags = *patch.Tags
	}
	if patch.Visibility != nil {
		tc.Visibility = *patch.Visibility
	}
	tc.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *fakeRepo) UpdateStatus(_ context.Context, id string, status testcase.Status, vis testcase.Visibility) error {
	tc, ok := r.store[id]
	if !ok {
		return testcase.ErrNotFound
	}
	tc.Status = status
	tc.Visibility = vis
	tc.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id string) (*testcase.TestCase, error) {
	tc, ok := r.store[id]
	if !ok {
		return nil, testcase.ErrNotFound
	}
	return tc, nil
}

// List applies a minimal subset of buildListFilter's authorization rules so
// service-level tests can exercise selector behaviour without depending on
// MongoDB. The implementation matches the documented invariants:
//   - Anonymous => public-published only.
//   - Admin => sees everything.
//   - Developer => public-published OR own.
//   - Selectors further narrow the candidate set.
func (r *fakeRepo) List(_ context.Context, f testcase.ListFilter) ([]*testcase.TestCase, error) {
	out := make([]*testcase.TestCase, 0, len(r.store))
	for _, tc := range r.store {
		if !visible(tc, f) {
			continue
		}
		if !matchesSelector(tc, f) {
			continue
		}
		if f.Search != "" && !strings.Contains(strings.ToLower(tc.ID+" "+tc.Name), strings.ToLower(f.Search)) {
			continue
		}
		out = append(out, tc)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (r *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.store[id]; !ok {
		return testcase.ErrNotFound
	}
	delete(r.store, id)
	return nil
}

type fakeArtifactRepo struct {
	artifacts map[string][]*domartifact.Artifact
	deleted   []string
}

func (r *fakeArtifactRepo) List(_ context.Context, caseID string) ([]*domartifact.Artifact, error) {
	return r.artifacts[caseID], nil
}

func (r *fakeArtifactRepo) Delete(_ context.Context, caseID, artifactName string) error {
	r.deleted = append(r.deleted, caseID+"/"+artifactName)
	return nil
}

type fakeObjectStore struct {
	deleted []string
}

func (s *fakeObjectStore) DeleteObject(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func visible(tc *testcase.TestCase, f testcase.ListFilter) bool {
	if f.CallerIsAdmin {
		return true
	}
	if tc.IsPubliclyReadable() {
		return true
	}
	if f.CallerUserID != "" && tc.OwnerUserID == f.CallerUserID {
		return true
	}
	return false
}

func matchesSelector(tc *testcase.TestCase, f testcase.ListFilter) bool {
	switch f.Selector {
	case testcase.ListSelectorMine:
		return f.CallerUserID != "" && tc.OwnerUserID == f.CallerUserID
	case testcase.ListSelectorDrafts:
		if !tc.IsDraft() {
			return false
		}
		if f.CallerIsAdmin {
			return true
		}
		return f.CallerUserID != "" && tc.OwnerUserID == f.CallerUserID
	case testcase.ListSelectorPublic:
		return tc.IsPublished() && tc.Visibility == testcase.VisibilityPublic
	case testcase.ListSelectorPrivate:
		if !(tc.IsPublished() && tc.Visibility == testcase.VisibilityPrivate) {
			return false
		}
		if f.CallerIsAdmin {
			return true
		}
		return f.CallerUserID != "" && tc.OwnerUserID == f.CallerUserID
	default: // all
		return true
	}
}

// --- create/get tests ---

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

func TestCreateMetadata_DefaultsToDraftPrivate(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, err := svc.CreateMetadata(context.Background(), testcase.MetadataInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.Status != testcase.StatusDraft {
		t.Errorf("expected status=draft, got %q", tc.Status)
	}
	if tc.Visibility != testcase.VisibilityPrivate {
		t.Errorf("expected visibility=private, got %q", tc.Visibility)
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

func TestCreateMetadata_NormalisesTags(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, err := svc.CreateMetadata(context.Background(), testcase.MetadataInput{
		Tags: []string{"  alpha ", "", "beta\t"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alpha", "beta"}
	if len(tc.Tags) != len(want) {
		t.Fatalf("expected %d tags, got %d", len(want), len(tc.Tags))
	}
	for i, w := range want {
		if tc.Tags[i] != w {
			t.Errorf("tag[%d]: want %q, got %q", i, w, tc.Tags[i])
		}
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

func TestDeleteTestCase_OwnerDeletesOwnCaseAndArtifacts(t *testing.T) {
	repo := newFakeRepo()
	artRepo := &fakeArtifactRepo{artifacts: map[string][]*domartifact.Artifact{
		"TC-owned": {
			{CaseID: "TC-owned", ArtifactName: "env.json", StorageKey: "testcases/TC-owned/artifacts/env.json"},
			{CaseID: "TC-owned", ArtifactName: "result.txt", StorageKey: "testcases/TC-owned/artifacts/result.txt"},
		},
	}}
	store := &fakeObjectStore{}
	svc := apptestcase.NewService(repo, apptestcase.DeleteCleanup{ArtifactRepo: artRepo, Store: store})
	if err := repo.Create(context.Background(), &testcase.TestCase{
		ID: "TC-owned", OwnerUserID: "usr1", Status: testcase.StatusDraft, Visibility: testcase.VisibilityPrivate,
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteTestCase(context.Background(), "TC-owned", "usr1", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := repo.FindByID(context.Background(), "TC-owned"); !errors.Is(err, testcase.ErrNotFound) {
		t.Fatalf("expected testcase to be deleted, got %v", err)
	}
	if len(store.deleted) != 2 {
		t.Fatalf("expected 2 object deletes, got %d", len(store.deleted))
	}
	if len(artRepo.deleted) != 2 {
		t.Fatalf("expected 2 artifact metadata deletes, got %d", len(artRepo.deleted))
	}
}

func TestDeleteTestCase_NonOwnerRejected(t *testing.T) {
	repo := newFakeRepo()
	svc := apptestcase.NewService(repo)
	if err := repo.Create(context.Background(), &testcase.TestCase{
		ID: "TC-owned", OwnerUserID: "usr1", Status: testcase.StatusDraft, Visibility: testcase.VisibilityPrivate,
	}); err != nil {
		t.Fatal(err)
	}

	err := svc.DeleteTestCase(context.Background(), "TC-owned", "usr2", false)
	if !errors.Is(err, testcase.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestDeleteTestCase_AdminDeletesLegacyCase(t *testing.T) {
	repo := newFakeRepo()
	svc := apptestcase.NewService(repo)
	if err := repo.Create(context.Background(), &testcase.TestCase{
		ID: "TC-legacy", Status: testcase.StatusPublished, Visibility: testcase.VisibilityPrivate,
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteTestCase(context.Background(), "TC-legacy", "admin", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
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

// --- publish flow tests ---

func TestPublish_DraftBecomesPublishedPublic(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	out, err := svc.Publish(context.Background(), tc.ID, "", "u1", false)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if out.Status != testcase.StatusPublished {
		t.Errorf("expected status=published, got %q", out.Status)
	}
	if out.Visibility != testcase.VisibilityPublic {
		t.Errorf("expected visibility=public, got %q", out.Visibility)
	}
}

func TestPublish_DraftToPrivate(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	out, err := svc.Publish(context.Background(), tc.ID, testcase.VisibilityPrivate, "u1", false)
	if err != nil {
		t.Fatalf("publish private: %v", err)
	}
	if out.Visibility != testcase.VisibilityPrivate {
		t.Errorf("expected visibility=private, got %q", out.Visibility)
	}
}

func TestPublish_AlreadyPublished_Rejects(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})
	_, _ = svc.Publish(context.Background(), tc.ID, testcase.VisibilityPublic, "u1", false)

	_, err := svc.Publish(context.Background(), tc.ID, testcase.VisibilityPublic, "u1", false)
	if !errors.Is(err, testcase.ErrNotDraft) {
		t.Errorf("expected ErrNotDraft, got %v", err)
	}
}

func TestPublish_NonOwner_Forbidden(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	_, err := svc.Publish(context.Background(), tc.ID, testcase.VisibilityPublic, "u2", false)
	if !errors.Is(err, testcase.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestPublish_AdminCanPublishAnyone(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	out, err := svc.Publish(context.Background(), tc.ID, testcase.VisibilityPublic, "admin", true)
	if err != nil {
		t.Fatalf("admin publish: %v", err)
	}
	if !out.IsPublished() {
		t.Error("expected published after admin publish")
	}
}

func TestPublish_InvalidVisibility(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	_, err := svc.Publish(context.Background(), tc.ID, testcase.Visibility("strange"), "u1", false)
	if err == nil {
		t.Error("expected error for invalid visibility")
	}
}

// --- update metadata tests ---

func TestUpdateMetadata_NameAndTags(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	newName := "updated"
	newTags := []string{"a", "b"}
	out, err := svc.UpdateMetadata(context.Background(), tc.ID, testcase.MetadataPatch{
		Name: &newName,
		Tags: &newTags,
	}, "u1", false)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.Name != "updated" {
		t.Errorf("name not applied")
	}
	if len(out.Tags) != 2 {
		t.Errorf("tags not applied")
	}
}

func TestUpdateMetadata_TestTypeOnDraftAllowed(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	newType := "sglang-benchmark"
	_, err := svc.UpdateMetadata(context.Background(), tc.ID, testcase.MetadataPatch{
		TestType: &newType,
	}, "u1", false)
	if err != nil {
		t.Fatalf("expected test_type change on draft to succeed, got %v", err)
	}
}

func TestUpdateMetadata_TestTypeOnPublishedRejected(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})
	_, _ = svc.Publish(context.Background(), tc.ID, testcase.VisibilityPublic, "u1", false)

	newType := "vllm-benchmark"
	_, err := svc.UpdateMetadata(context.Background(), tc.ID, testcase.MetadataPatch{
		TestType: &newType,
	}, "u1", false)
	if !errors.Is(err, testcase.ErrNotDraft) {
		t.Errorf("expected ErrNotDraft, got %v", err)
	}
}

func TestUpdateMetadata_VisibilityOnDraftRejected(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	v := testcase.VisibilityPublic
	_, err := svc.UpdateMetadata(context.Background(), tc.ID, testcase.MetadataPatch{
		Visibility: &v,
	}, "u1", false)
	if !errors.Is(err, testcase.ErrNotPublished) {
		t.Errorf("expected ErrNotPublished, got %v", err)
	}
}

func TestUpdateMetadata_VisibilityOnPublishedAllowed(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})
	_, _ = svc.Publish(context.Background(), tc.ID, testcase.VisibilityPublic, "u1", false)

	v := testcase.VisibilityPrivate
	out, err := svc.UpdateMetadata(context.Background(), tc.ID, testcase.MetadataPatch{
		Visibility: &v,
	}, "u1", false)
	if err != nil {
		t.Fatalf("expected visibility change on published, got %v", err)
	}
	if out.Visibility != testcase.VisibilityPrivate {
		t.Errorf("expected visibility=private, got %q", out.Visibility)
	}
}

func TestUpdateMetadata_NonOwner_Forbidden(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())
	tc, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	newName := "spy"
	_, err := svc.UpdateMetadata(context.Background(), tc.ID, testcase.MetadataPatch{
		Name: &newName,
	}, "u2", false)
	if !errors.Is(err, testcase.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

// --- list tests ---

func TestList_AnonymousSeesPublicPublishedOnly(t *testing.T) {
	repo := newFakeRepo()
	svc := apptestcase.NewService(repo)

	// public published
	tcA, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})
	_, _ = svc.Publish(context.Background(), tcA.ID, testcase.VisibilityPublic, "u1", false)
	// private published
	tcB, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})
	_, _ = svc.Publish(context.Background(), tcB.ID, testcase.VisibilityPrivate, "u1", false)
	// draft
	_, _ = svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})

	out, err := svc.ListTestCases(context.Background(), testcase.ListFilter{Selector: testcase.ListSelectorAll})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 visible to anonymous, got %d", len(out))
	}
	if out[0].ID != tcA.ID {
		t.Errorf("expected to see %s, got %s", tcA.ID, out[0].ID)
	}
}

func TestList_DeveloperSeesOwnAndPublic(t *testing.T) {
	repo := newFakeRepo()
	svc := apptestcase.NewService(repo)

	// public published by other user
	tcA, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u2"})
	_, _ = svc.Publish(context.Background(), tcA.ID, testcase.VisibilityPublic, "u2", false)
	// caller's own draft
	tcB, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})
	// other user's private — should NOT appear
	tcC, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u2"})
	_, _ = svc.Publish(context.Background(), tcC.ID, testcase.VisibilityPrivate, "u2", false)

	out, err := svc.ListTestCases(context.Background(), testcase.ListFilter{
		Selector:     testcase.ListSelectorAll,
		CallerUserID: "u1",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]bool{}
	for _, tc := range out {
		got[tc.ID] = true
	}
	if !got[tcA.ID] || !got[tcB.ID] {
		t.Errorf("expected own+public visible, got %v", got)
	}
	if got[tcC.ID] {
		t.Errorf("private of another user must not appear: %v", got)
	}
}

func TestList_AdminSeesAll(t *testing.T) {
	repo := newFakeRepo()
	svc := apptestcase.NewService(repo)

	_, _ = svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u1"})
	tcB, _ := svc.CreateMetadata(context.Background(), testcase.MetadataInput{OwnerUserID: "u2"})
	_, _ = svc.Publish(context.Background(), tcB.ID, testcase.VisibilityPrivate, "u2", false)

	out, _ := svc.ListTestCases(context.Background(), testcase.ListFilter{
		Selector:      testcase.ListSelectorAll,
		CallerIsAdmin: true,
	})
	if len(out) != 2 {
		t.Errorf("expected admin to see all 2, got %d", len(out))
	}
}

// --- CanRead tests ---

func TestCanRead(t *testing.T) {
	svc := apptestcase.NewService(newFakeRepo())

	draft := &testcase.TestCase{ID: "d", Status: testcase.StatusDraft, Visibility: testcase.VisibilityPrivate, OwnerUserID: "u1"}
	pub := &testcase.TestCase{ID: "p", Status: testcase.StatusPublished, Visibility: testcase.VisibilityPublic, OwnerUserID: "u1"}
	priv := &testcase.TestCase{ID: "v", Status: testcase.StatusPublished, Visibility: testcase.VisibilityPrivate, OwnerUserID: "u1"}

	if svc.CanRead(draft, "", false) {
		t.Error("anonymous should not read draft")
	}
	if !svc.CanRead(pub, "", false) {
		t.Error("anonymous should read public-published")
	}
	if svc.CanRead(priv, "", false) {
		t.Error("anonymous should not read published-private")
	}
	if !svc.CanRead(priv, "u1", false) {
		t.Error("owner should read own published-private")
	}
	if svc.CanRead(priv, "u2", false) {
		t.Error("non-owner developer should not read another's private")
	}
	if !svc.CanRead(priv, "u2", true) {
		t.Error("admin should read any testcase")
	}
}
