package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apptestcase "github.com/AFDEAPAC/kish/internal/application/testcase"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
	infrahttp "github.com/AFDEAPAC/kish/internal/interfaces/http"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/handler"
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

func (r *fakeRepo) FindByID(_ context.Context, id string) (*testcase.TestCase, error) {
	tc, ok := r.store[id]
	if !ok {
		return nil, testcase.ErrNotFound
	}
	return tc, nil
}

func newTestServer() *httptest.Server {
	repo := newFakeRepo()
	svc := apptestcase.NewService(repo)
	mux := http.NewServeMux()
	infrahttp.RegisterRoutes(mux, handler.NewHealthHandler(), handler.NewTestCaseHandler(svc), handler.NewArtifactHandler(nil))
	return httptest.NewServer(mux)
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
}
