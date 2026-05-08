// Internal test package so we can access unexported helpers directly.
package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AFDEAPAC/kish/internal/application/detect"
	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

type fakeDetectService struct {
	snapshot *environment.EnvironmentSnapshot
	scopes   *[]environment.EnvironmentScope
}

func (s *fakeDetectService) DetectWithScope(_ context.Context, _ detect.Reporter, scope environment.EnvironmentScope) (*environment.EnvironmentSnapshot, error) {
	*s.scopes = append(*s.scopes, scope)
	snapshot := *s.snapshot
	snapshot.Scope = scope
	return &snapshot, nil
}

func installFakeDetectService(t *testing.T, snapshot *environment.EnvironmentSnapshot) *[]environment.EnvironmentScope {
	t.Helper()
	origRunner := newDetectCommandRunner
	origCollectors := newDetectCollectors
	origService := newDetectService
	scopes := []environment.EnvironmentScope{}

	newDetectCommandRunner = func() command.Runner { return nil }
	newDetectCollectors = func(_ command.Runner, _ string) []environment.Collector { return nil }
	newDetectService = func(_ []environment.Collector) detectService {
		return &fakeDetectService{snapshot: snapshot, scopes: &scopes}
	}
	t.Cleanup(func() {
		newDetectCommandRunner = origRunner
		newDetectCollectors = origCollectors
		newDetectService = origService
	})
	return &scopes
}

func testSnapshot() *environment.EnvironmentSnapshot {
	return &environment.EnvironmentSnapshot{
		SchemaVersion: environment.SchemaVersionV1,
		KishVersion:   "test",
		Scope:         environment.ScopeExecution,
		Type:          environment.TypeHost,
		CollectedAt:   time.Date(2026, 5, 8, 1, 2, 3, 0, time.UTC),
		Data:          map[string]string{"hostname": "test-host"},
		PackageSets:   []environment.PackageSet{},
		Raw:           map[string]string{},
		Collectors:    []environment.CollectorReport{},
	}
}

func TestDefaultOutputPath_Format(t *testing.T) {
	ts := time.Date(2026, 4, 29, 15, 30, 22, 0, time.UTC)

	cases := []struct {
		hostname string
		want     string
	}{
		{"gpunode01", "env_gpunode01_20260429153022.json"},
		{"my-host.local", "env_my-host.local_20260429153022.json"},
		{"host with spaces", "env_host_with_spaces_20260429153022.json"},
		{"", "env_unknown-host_20260429153022.json"},
	}

	for _, tc := range cases {
		got := defaultOutputPath(tc.hostname, ts)
		if got != tc.want {
			t.Errorf("defaultOutputPath(%q, ts) = %q, want %q", tc.hostname, got, tc.want)
		}
	}
}

func TestSanitizeHostname(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"gpunode01", "gpunode01"},
		{"my-host.local", "my-host.local"},
		{"host_name", "host_name"},
		{"host with spaces", "host_with_spaces"},
		{"host@domain!com", "host_domain_com"},
		{"", ""},
	}
	for _, tc := range cases {
		got := sanitizeHostname(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeHostname(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDefaultOutputPath_ContainsExpectedParts(t *testing.T) {
	ts := time.Date(2026, 4, 29, 15, 30, 22, 0, time.UTC)
	result := defaultOutputPath("myhost", ts)
	if !strings.HasPrefix(result, "env_myhost_") {
		t.Errorf("expected prefix env_myhost_, got %q", result)
	}
	if !strings.HasSuffix(result, ".json") {
		t.Errorf("expected .json suffix, got %q", result)
	}
	if !strings.Contains(result, "20260429153022") {
		t.Errorf("expected timestamp 20260429153022 in %q", result)
	}
}

func TestRunDetect_WithoutCaseIDWritesOutputAndDoesNotUpload(t *testing.T) {
	scopes := installFakeDetectService(t, testSnapshot())
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	outputPath := filepath.Join(t.TempDir(), "env.json")
	err := runDetect(context.Background(), detectFlags{
		output:  outputPath,
		apiBase: srv.URL,
		token:   "kish_test",
		quiet:   true,
	})
	if err != nil {
		t.Fatalf("run detect: %v", err)
	}
	if called.Load() {
		t.Fatal("expected no API call without --case-id")
	}
	if len(*scopes) != 1 || (*scopes)[0] != environment.ScopeExecution {
		t.Fatalf("expected execution scope, got %#v", *scopes)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("expected output file: %v", err)
	}
	var snapshot environment.EnvironmentSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapshot.Scope != environment.ScopeExecution {
		t.Fatalf("expected execution scope in output, got %q", snapshot.Scope)
	}
}

func TestRunDetect_WithCaseIDUploadsInMemorySnapshot(t *testing.T) {
	installFakeDetectService(t, testSnapshot())
	var putCalled atomic.Bool
	var gotAuth, gotType, gotContentType, gotPath, gotScope string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		putCalled.Store(true)
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("X-Kish-Artifact-Type")
		gotContentType = r.Header.Get("Content-Type")
		var snapshot environment.EnvironmentSnapshot
		if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
			t.Fatalf("decode uploaded snapshot: %v", err)
		}
		gotScope = string(snapshot.Scope)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })

	err = runDetect(context.Background(), detectFlags{
		caseID:  "TC-001",
		apiBase: srv.URL,
		token:   "kish_test",
		scope:   "supporting",
		quiet:   true,
	})
	if err != nil {
		t.Fatalf("run detect: %v", err)
	}
	if !putCalled.Load() {
		t.Fatal("expected PUT upload")
	}
	if gotPath != "/api/v1/testcases/TC-001/artifacts/env_test-host_20260508010203.json" {
		t.Fatalf("unexpected upload path: %s", gotPath)
	}
	if gotAuth != "Bearer kish_test" {
		t.Fatalf("unexpected auth header: %q", gotAuth)
	}
	if gotType != "environment" {
		t.Fatalf("unexpected artifact type: %q", gotType)
	}
	if gotContentType != "application/json" {
		t.Fatalf("unexpected content type: %q", gotContentType)
	}
	if gotScope != "supporting" {
		t.Fatalf("expected supporting scope, got %q", gotScope)
	}
	if _, err := os.Stat(filepath.Join(dir, "env_test-host_20260508010203.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no auto-generated output file when --case-id is used without --output, stat err=%v", err)
	}
}

func TestRunDetect_WithCaseIDRequiresAPIBase(t *testing.T) {
	t.Setenv(kishAPIURLEnv, "")
	installFakeDetectService(t, testSnapshot())
	err := runDetect(context.Background(), detectFlags{caseID: "TC-001", token: "kish_test", quiet: true})
	if err == nil || !strings.Contains(err.Error(), "--api is required") {
		t.Fatalf("expected missing api error, got %v", err)
	}
}

func TestRunDetect_WithCaseIDRequiresToken(t *testing.T) {
	t.Setenv(kishAPITokenEnv, "")
	installFakeDetectService(t, testSnapshot())
	err := runDetect(context.Background(), detectFlags{caseID: "TC-001", apiBase: "http://127.0.0.1:30151", quiet: true})
	if err == nil || !strings.Contains(err.Error(), "--token is required") {
		t.Fatalf("expected missing token error, got %v", err)
	}
}

func TestRunDetect_InvalidScopeSkipsDetection(t *testing.T) {
	scopes := installFakeDetectService(t, testSnapshot())
	err := runDetect(context.Background(), detectFlags{scope: "invalid", quiet: true})
	if err == nil || !strings.Contains(err.Error(), "allowed values: execution, supporting") {
		t.Fatalf("expected invalid scope error, got %v", err)
	}
	if len(*scopes) != 0 {
		t.Fatalf("expected detection not to run, got scopes %#v", *scopes)
	}
}

func TestRunDetect_UploadFailureKeepsOutputFile(t *testing.T) {
	installFakeDetectService(t, testSnapshot())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	outputPath := filepath.Join(t.TempDir(), "env.json")
	err := runDetect(context.Background(), detectFlags{
		output:  outputPath,
		caseID:  "TC-001",
		apiBase: srv.URL,
		token:   "kish_test",
		quiet:   true,
	})
	if err == nil || !strings.Contains(err.Error(), "snapshot written to") {
		t.Fatalf("expected upload failure with written snapshot note, got %v", err)
	}
	if _, statErr := os.Stat(outputPath); statErr != nil {
		t.Fatalf("expected output file to remain after upload failure: %v", statErr)
	}
}
