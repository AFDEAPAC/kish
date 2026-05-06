package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appArtifact "github.com/AFDEAPAC/kish/internal/application/artifact"
	domArtifact "github.com/AFDEAPAC/kish/internal/domain/artifact"
	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/infrastructure/storage"
	infrahttp "github.com/AFDEAPAC/kish/internal/interfaces/http"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/handler"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// --- shared fakes (same pattern as service_test.go) ---

type artHandlerFakeTCRepo struct {
	ids map[string]bool
}

func newArtHandlerTCRepo(ids ...string) *artHandlerFakeTCRepo {
	r := &artHandlerFakeTCRepo{ids: make(map[string]bool)}
	for _, id := range ids {
		r.ids[id] = true
	}
	return r
}
func (r *artHandlerFakeTCRepo) Create(_ context.Context, tc *testcase.TestCase) error {
	r.ids[tc.ID] = true
	return nil
}
func (r *artHandlerFakeTCRepo) Update(_ context.Context, id string, _ *testcase.TestCase) error {
	if !r.ids[id] {
		return testcase.ErrNotFound
	}
	return nil
}
func (r *artHandlerFakeTCRepo) FindByID(_ context.Context, id string) (*testcase.TestCase, error) {
	if !r.ids[id] {
		return nil, testcase.ErrNotFound
	}
	return &testcase.TestCase{
		ID: id, TestType: "generic",
		Environment: &environment.EnvironmentSnapshot{SchemaVersion: environment.SchemaVersionV1},
		ResultArtifact: testcase.Artifact{Content: "x"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}, nil
}

type artHandlerFakeArtRepo struct {
	store map[string]*domArtifact.Artifact
}

func newArtHandlerArtRepo() *artHandlerFakeArtRepo {
	return &artHandlerFakeArtRepo{store: make(map[string]*domArtifact.Artifact)}
}
func repoKey(caseID, name string) string { return caseID + "||" + name }
func (r *artHandlerFakeArtRepo) Upsert(_ context.Context, a *domArtifact.Artifact) error {
	if ex, ok := r.store[repoKey(a.CaseID, a.ArtifactName)]; ok {
		a.CreatedAt = ex.CreatedAt
	}
	r.store[repoKey(a.CaseID, a.ArtifactName)] = a
	return nil
}
func (r *artHandlerFakeArtRepo) FindOne(_ context.Context, caseID, name string) (*domArtifact.Artifact, error) {
	a, ok := r.store[repoKey(caseID, name)]
	if !ok {
		return nil, domArtifact.ErrNotFound
	}
	return a, nil
}
func (r *artHandlerFakeArtRepo) List(_ context.Context, caseID string) ([]*domArtifact.Artifact, error) {
	var out []*domArtifact.Artifact
	for k, v := range r.store {
		if strings.HasPrefix(k, caseID+"||") {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *artHandlerFakeArtRepo) Delete(_ context.Context, caseID, name string) error {
	delete(r.store, repoKey(caseID, name))
	return nil
}

type artHandlerFakeStore struct {
	objects map[string][]byte
}

func newArtHandlerStore() *artHandlerFakeStore {
	return &artHandlerFakeStore{objects: make(map[string][]byte)}
}
func (s *artHandlerFakeStore) PutObject(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, _ := io.ReadAll(r)
	s.objects[key] = data
	return nil
}
func (s *artHandlerFakeStore) GetObject(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, storage.ObjectInfo{}, storage.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), storage.ObjectInfo{Key: key, Size: int64(len(data))}, nil
}
func (s *artHandlerFakeStore) DeleteObject(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

// adminPrincipalMiddleware injects an admin principal into every request context.
// Used in tests to bypass the real auth middleware.
func adminPrincipalMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := middleware.Principal{
			UserID:     "admin-test-user",
			Role:       user.RoleAdmin,
			AuthMethod: middleware.AuthMethodJWT,
		}
		h.ServeHTTP(w, r.WithContext(middleware.WithPrincipal(r.Context(), p)))
	})
}

func newArtTestServer(tcIDs ...string) *httptest.Server {
	tcRepo := newArtHandlerTCRepo(tcIDs...)
	artRepo := newArtHandlerArtRepo()
	objStore := newArtHandlerStore()
	artSvc := appArtifact.NewService(tcRepo, artRepo, objStore)

	mux := http.NewServeMux()
	infrahttp.RegisterRoutes(mux,
		handler.NewHealthHandler(),
		handler.NewTestCaseHandler(nil), // testcase handler not under test here
		handler.NewArtifactHandler(artSvc),
		handler.NewAuthHandler(nil, nil),
		handler.NewUserHandler(nil),
		handler.NewMeHandler(nil),
		handler.NewClientTokenHandler(nil),
		adminPrincipalMiddleware,
	)
	return httptest.NewServer(infrahttp.WrapWithAuth(mux, adminPrincipalMiddleware))
}

func TestArtifactPut_Success(t *testing.T) {
	srv := newArtTestServer("tc1")
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/testcases/tc1/artifacts/result.txt", strings.NewReader("benchmark output"))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Kish-Artifact-Type", "result")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var out dto.ArtifactMetaResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ArtifactName != "result.txt" {
		t.Errorf("unexpected artifact_name: %q", out.ArtifactName)
	}
	if out.ArtifactType != "result" {
		t.Errorf("unexpected artifact_type: %q", out.ArtifactType)
	}
	// StorageKey must not be present in the response JSON.
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "storage_key") {
		t.Error("response must not contain storage_key")
	}
}

func TestArtifactPut_TestCaseNotFound(t *testing.T) {
	srv := newArtTestServer() // no cases
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/testcases/missing/artifacts/result.txt", strings.NewReader("data"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestArtifactPut_InvalidName(t *testing.T) {
	srv := newArtTestServer("tc1")
	defer srv.Close()

	// "../escape" contains ".." which will be rejected by path routing before reaching
	// the handler, so use a name with invalid chars that the router passes through.
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/testcases/tc1/artifacts/file%20name", strings.NewReader("data"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid name, got %d", resp.StatusCode)
	}
}

func TestArtifactGet_Success(t *testing.T) {
	srv := newArtTestServer("tc1")
	defer srv.Close()

	// Upload first.
	putReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/testcases/tc1/artifacts/env.json", strings.NewReader(`{"schema_version":"test"}`))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, _ := http.DefaultClient.Do(putReq)
	putResp.Body.Close()

	// Download.
	getResp, err := http.Get(srv.URL + "/api/v1/testcases/tc1/artifacts/env.json")
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", getResp.StatusCode)
	}
	body, _ := io.ReadAll(getResp.Body)
	if !strings.Contains(string(body), "schema_version") {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestArtifactGet_NotFound(t *testing.T) {
	srv := newArtTestServer("tc1")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/testcases/tc1/artifacts/missing.txt")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestArtifactList_ReturnsArtifacts(t *testing.T) {
	srv := newArtTestServer("tc1")
	defer srv.Close()

	for _, name := range []string{"a.txt", "b.txt"} {
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/testcases/tc1/artifacts/"+name, strings.NewReader("x"))
		req.Header.Set("Content-Type", "text/plain")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	resp, err := http.Get(srv.URL + "/api/v1/testcases/tc1/artifacts")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var out dto.ListArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.CaseID != "tc1" {
		t.Errorf("unexpected case_id: %q", out.CaseID)
	}
	if len(out.Artifacts) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(out.Artifacts))
	}
}

func TestArtifactDelete_Success(t *testing.T) {
	srv := newArtTestServer("tc1")
	defer srv.Close()

	// Upload.
	putReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/testcases/tc1/artifacts/del.txt", strings.NewReader("bye"))
	putReq.Header.Set("Content-Type", "text/plain")
	putResp, _ := http.DefaultClient.Do(putReq)
	putResp.Body.Close()

	// Delete.
	delReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/testcases/tc1/artifacts/del.txt", nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", delResp.StatusCode)
	}

	// Confirm gone.
	getResp, _ := http.Get(srv.URL + "/api/v1/testcases/tc1/artifacts/del.txt")
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}
