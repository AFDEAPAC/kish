package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apptestcase "github.com/AFDEAPAC/kish/internal/application/testcase"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	infrahttp "github.com/AFDEAPAC/kish/internal/interfaces/http"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/handler"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// fakeRepo is a minimal in-memory repository for handler tests.
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

// Update is required by the testcase.Repository interface.
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
	return nil
}

func (r *fakeRepo) UpdateStatus(_ context.Context, id string, status testcase.Status, vis testcase.Visibility) error {
	tc, ok := r.store[id]
	if !ok {
		return testcase.ErrNotFound
	}
	tc.Status = status
	tc.Visibility = vis
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id string) (*testcase.TestCase, error) {
	tc, ok := r.store[id]
	if !ok {
		return nil, testcase.ErrNotFound
	}
	return tc, nil
}

// List returns every stored TestCase. The handler test fixture uses the admin
// principal so visibility filtering is a no-op; selector filtering is exercised
// in the application-layer tests.
func (r *fakeRepo) List(_ context.Context, _ testcase.ListFilter) ([]*testcase.TestCase, error) {
	out := make([]*testcase.TestCase, 0, len(r.store))
	for _, tc := range r.store {
		out = append(out, tc)
	}
	return out, nil
}

func (r *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.store[id]; !ok {
		return testcase.ErrNotFound
	}
	delete(r.store, id)
	return nil
}

type fakeOwnerResolver struct {
	users map[string]*user.User
	err   error
}

func (r fakeOwnerResolver) GetUser(_ context.Context, id string) (*user.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	u, ok := r.users[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}

// testAdminMiddleware injects an admin principal so protected endpoints work in tests.
func testAdminMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := middleware.Principal{
			UserID:     "test-admin",
			Role:       user.RoleAdmin,
			AuthMethod: middleware.AuthMethodJWT,
		}
		h.ServeHTTP(w, r.WithContext(middleware.WithPrincipal(r.Context(), p)))
	})
}

func newTestServer() *httptest.Server {
	return newTestServerWithOwnerResolver(nil)
}

func newTestServerWithOwnerResolver(ownerResolver interface {
	GetUser(context.Context, string) (*user.User, error)
}) *httptest.Server {
	repo := newFakeRepo()
	svc := apptestcase.NewService(repo)
	mux := http.NewServeMux()
	infrahttp.RegisterRoutes(mux,
		handler.NewHealthHandler(),
		handler.NewTestCaseHandler(svc, ownerResolver),
		handler.NewArtifactHandler(nil),
		handler.NewAuthHandler(nil, nil),
		handler.NewUserHandler(nil),
		handler.NewMeHandler(nil),
		handler.NewClientTokenHandler(nil),
	)
	return httptest.NewServer(infrahttp.WrapWithAuth(mux, testAdminMiddleware))
}

func TestHealthz_ReturnsOK(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body dto.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Errorf("expected status=ok, got %q", body.Status)
	}
}

func TestCreateV1TestCase_Returns201(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	req := dto.CreateTestCaseV1Request{
		Name:     "my test",
		TestType: "generic",
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(srv.URL+"/api/v1/testcases", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var out dto.CreateTestCaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.CaseID == "" {
		t.Error("expected non-empty case_id")
	}
	if !strings.HasPrefix(out.CaseID, "TC-") {
		t.Errorf("expected case_id to start with TC-, got %q", out.CaseID)
	}
	if !out.Created {
		t.Error("expected created=true")
	}
}

func TestCreateV1TestCase_DefaultsTestType(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	req := dto.CreateTestCaseV1Request{}
	body, _ := json.Marshal(req)
	resp, err := http.Post(srv.URL+"/api/v1/testcases", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 for empty body, got %d", resp.StatusCode)
	}
}

func TestCreateV1TestCase_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/testcases", "application/json", strings.NewReader("{invalid json"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestGetTestCase_NotFound_Returns404(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/testcases/TC-doesnotexist")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetTestCase_ReturnsCreatedCase(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	// Create a testcase via v1 metadata endpoint.
	createBody, _ := json.Marshal(dto.CreateTestCaseV1Request{TestType: "generic"})
	createResp, err := http.Post(srv.URL+"/api/v1/testcases", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	var created dto.CreateTestCaseResponse
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	// Retrieve it.
	getResp, err := http.Get(srv.URL + "/api/testcases/" + created.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", getResp.StatusCode)
	}

	var out dto.TestCaseResponse
	if err := json.NewDecoder(getResp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ID != created.CaseID {
		t.Errorf("expected id=%q, got %q", created.CaseID, out.ID)
	}
	if out.Status != string(testcase.StatusDraft) {
		t.Errorf("expected status=draft on new TestCase, got %q", out.Status)
	}
	if out.Visibility != string(testcase.VisibilityPrivate) {
		t.Errorf("expected visibility=private on new TestCase, got %q", out.Visibility)
	}
}

// createTestCase posts a new TestCase metadata record via the test server and
// returns its case_id.
func createTestCase(t *testing.T, srvURL string) string {
	t.Helper()
	body, _ := json.Marshal(dto.CreateTestCaseV1Request{TestType: "generic"})
	resp, err := http.Post(srvURL+"/api/v1/testcases", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out dto.CreateTestCaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.CaseID
}

func TestPublishTestCase_DraftBecomesPublishedPublic(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	caseID := createTestCase(t, srv.URL)

	body, _ := json.Marshal(dto.PublishTestCaseRequest{Visibility: "public"})
	resp, err := http.Post(srv.URL+"/api/v1/testcases/"+caseID+"/publish", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out dto.TestCaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != string(testcase.StatusPublished) {
		t.Errorf("expected published, got %q", out.Status)
	}
	if out.Visibility != string(testcase.VisibilityPublic) {
		t.Errorf("expected public, got %q", out.Visibility)
	}
}

func TestPublishTestCase_DefaultsToPublic_OnEmptyBody(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	caseID := createTestCase(t, srv.URL)

	resp, err := http.Post(srv.URL+"/api/v1/testcases/"+caseID+"/publish", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out dto.TestCaseResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Visibility != string(testcase.VisibilityPublic) {
		t.Errorf("empty body should default to public, got %q", out.Visibility)
	}
}

func TestPublishTestCase_AlreadyPublished_409(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	caseID := createTestCase(t, srv.URL)

	_, _ = http.Post(srv.URL+"/api/v1/testcases/"+caseID+"/publish", "application/json", nil)
	resp, err := http.Post(srv.URL+"/api/v1/testcases/"+caseID+"/publish", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 on second publish, got %d", resp.StatusCode)
	}
}

func TestPatchTestCase_NameAndDescription(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	caseID := createTestCase(t, srv.URL)

	patchName := "renamed"
	patchDesc := "updated description"
	body, _ := json.Marshal(dto.UpdateTestCaseRequest{
		Name:        &patchName,
		Description: &patchDesc,
	})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/testcases/"+caseID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out dto.TestCaseResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Name != "renamed" {
		t.Errorf("name not applied: %q", out.Name)
	}
	if out.Description != "updated description" {
		t.Errorf("description not applied: %q", out.Description)
	}
}

func TestPatchTestCase_VisibilityOnDraft_409(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	caseID := createTestCase(t, srv.URL)

	v := "public"
	body, _ := json.Marshal(dto.UpdateTestCaseRequest{Visibility: &v})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/testcases/"+caseID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 changing visibility on draft, got %d", resp.StatusCode)
	}
}

func TestListTestCases_ReturnsCreated(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	caseID := createTestCase(t, srv.URL)

	resp, err := http.Get(srv.URL + "/api/v1/testcases")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out dto.ListTestCasesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tc := range out.TestCases {
		if tc.ID == caseID {
			found = true
		}
	}
	if !found {
		t.Errorf("created TestCase %s not in list", caseID)
	}
}

func TestGetV1TestCase_ReturnsCreated(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	caseID := createTestCase(t, srv.URL)

	resp, err := http.Get(srv.URL + "/api/v1/testcases/" + caseID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetV1TestCase_IncludesOwnerDisplayName(t *testing.T) {
	srv := newTestServerWithOwnerResolver(fakeOwnerResolver{
		users: map[string]*user.User{
			"test-admin": {ID: "test-admin", DisplayName: "Kish Admin"},
		},
	})
	defer srv.Close()
	caseID := createTestCase(t, srv.URL)

	resp, err := http.Get(srv.URL + "/api/v1/testcases/" + caseID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out dto.TestCaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.OwnerDisplayName != "Kish Admin" {
		t.Errorf("expected owner display name, got %q", out.OwnerDisplayName)
	}
}

func TestListTestCases_OwnerLookupFailureStillReturnsTestCases(t *testing.T) {
	srv := newTestServerWithOwnerResolver(fakeOwnerResolver{err: errors.New("lookup failed")})
	defer srv.Close()
	caseID := createTestCase(t, srv.URL)

	resp, err := http.Get(srv.URL + "/api/v1/testcases")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out dto.ListTestCasesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	for _, tc := range out.TestCases {
		if tc.ID == caseID {
			if tc.OwnerDisplayName != "" {
				t.Errorf("expected empty owner display name on lookup failure, got %q", tc.OwnerDisplayName)
			}
			return
		}
	}
	t.Fatalf("created TestCase %s not in list", caseID)
}
