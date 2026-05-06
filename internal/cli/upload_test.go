// Internal test package to access unexported upload helpers directly.
package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
)

// --- readTextFile tests (unchanged helper) ---

func TestReadTextFile_FileNotFound(t *testing.T) {
	_, err := readTextFile("/nonexistent/path/file.txt", 1024)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestReadTextFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.txt")
	content := "benchmark result\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := readTextFile(path, 1024*1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("expected %q, got %q", content, got)
	}
}

func TestReadTextFile_ExceedsMaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	data := make([]byte, 100)
	for i := range data {
		data[i] = 'x'
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := readTextFile(path, 10)
	if err == nil {
		t.Error("expected error when file exceeds maxBytes")
	}
}

// --- buildUploadArtifacts tests ---

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildUploadArtifacts_Success(t *testing.T) {
	dir := t.TempDir()
	envPath := writeTemp(t, dir, "env.json", `{"schema_version":"environment-snapshot/v1"}`)
	resultPath := writeTemp(t, dir, "result.txt", "output")
	scriptPath := writeTemp(t, dir, "run.sh", "#!/bin/bash")

	flags := uploadFlags{
		envFile: envPath,
		result:  resultPath,
		scripts: []string{scriptPath},
	}
	artifacts, err := buildUploadArtifacts(flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(artifacts))
	}
	if artifacts[0].artifactType != "environment" {
		t.Errorf("expected first artifact type=environment, got %q", artifacts[0].artifactType)
	}
	if artifacts[1].artifactType != "result" {
		t.Errorf("expected second artifact type=result, got %q", artifacts[1].artifactType)
	}
	if artifacts[2].artifactType != "script" {
		t.Errorf("expected third artifact type=script, got %q", artifacts[2].artifactType)
	}
}

func TestBuildUploadArtifacts_InvalidEnvJSON(t *testing.T) {
	dir := t.TempDir()
	envPath := writeTemp(t, dir, "env.json", `not json`)
	resultPath := writeTemp(t, dir, "result.txt", "output")

	_, err := buildUploadArtifacts(uploadFlags{envFile: envPath, result: resultPath})
	if err == nil {
		t.Error("expected error for invalid JSON env file")
	}
	if !strings.Contains(err.Error(), "--env") {
		t.Errorf("expected error to mention --env, got: %v", err)
	}
}

func TestBuildUploadArtifacts_MissingResult(t *testing.T) {
	dir := t.TempDir()
	envPath := writeTemp(t, dir, "env.json", `{}`)
	_, err := buildUploadArtifacts(uploadFlags{
		envFile: envPath,
		result:  filepath.Join(dir, "nonexistent.txt"),
	})
	if err == nil {
		t.Error("expected error for missing result file")
	}
}

func TestBuildUploadArtifacts_ArtifactNamesAreBasenames(t *testing.T) {
	dir := t.TempDir()
	envPath := writeTemp(t, dir, "env.json", `{}`)
	resultPath := writeTemp(t, dir, "result.txt", "data")

	artifacts, err := buildUploadArtifacts(uploadFlags{envFile: envPath, result: resultPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, a := range artifacts {
		if strings.Contains(a.artifactName, "/") || strings.Contains(a.artifactName, "\\") {
			t.Errorf("artifact_name should be a basename, got %q", a.artifactName)
		}
	}
}

func TestResolveAPIBase_UsesFlagFirst(t *testing.T) {
	t.Setenv("KISH_API_URL", "http://env.example")
	got := resolveAPIBase(uploadFlags{apiBase: "http://flag.example"})
	if got != "http://flag.example" {
		t.Fatalf("expected flag value, got %q", got)
	}
}

func TestResolveAPIBase_UsesEnvironmentFallback(t *testing.T) {
	t.Setenv("KISH_API_URL", "http://env.example")
	got := resolveAPIBase(uploadFlags{})
	if got != "http://env.example" {
		t.Fatalf("expected env value, got %q", got)
	}
}

func TestRunUpload_RequiresAPIBaseFromFlagOrEnv(t *testing.T) {
	t.Setenv("KISH_API_URL", "")
	dir := t.TempDir()
	envPath := writeTemp(t, dir, "env.json", `{}`)
	resultPath := writeTemp(t, dir, "result.txt", "data")

	err := runUpload(uploadFlags{envFile: envPath, result: resultPath})
	if err == nil || !strings.Contains(err.Error(), "KISH_API_URL") {
		t.Fatalf("expected missing API URL error, got %v", err)
	}
}

// --- ensureCaseID tests ---

func TestEnsureCaseID_WithCaseID_NoAPICalled(t *testing.T) {
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer srv.Close()

	flags := uploadFlags{apiBase: srv.URL, caseID: "TC-existing"}
	id, created, err := ensureCaseID(flags, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "TC-existing" {
		t.Errorf("expected id=TC-existing, got %q", id)
	}
	if created {
		t.Error("expected created=false when case-id is provided")
	}
	if called.Load() {
		t.Error("expected no API call when --case-id is provided")
	}
}

func TestEnsureCaseID_WithoutCaseID_CallsCreateEndpoint(t *testing.T) {
	var postCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/testcases" {
			postCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(dto.CreateTestCaseResponse{ //nolint:errcheck
				CaseID:  "TC-new-abc",
				Created: true,
			})
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	flags := uploadFlags{apiBase: srv.URL, testType: "generic"}
	id, created, err := ensureCaseID(flags, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "TC-new-abc" {
		t.Errorf("expected id=TC-new-abc, got %q", id)
	}
	if !created {
		t.Error("expected created=true when no case-id was provided")
	}
	if postCount.Load() != 1 {
		t.Errorf("expected 1 POST call, got %d", postCount.Load())
	}
}

// --- runUpload integration tests ---

func TestRunUpload_WithoutCaseID_CreatesAndUploads(t *testing.T) {
	dir := t.TempDir()
	envPath := writeTemp(t, dir, "env.json", `{"schema_version":"environment-snapshot/v1"}`)
	resultPath := writeTemp(t, dir, "result.txt", "bench output")
	scriptPath := writeTemp(t, dir, "run.sh", "#!/bin/bash")

	var postCalled atomic.Bool
	var putNames []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/testcases":
			postCalled.Store(true)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(dto.CreateTestCaseResponse{CaseID: "TC-test-123", Created: true}) //nolint:errcheck

		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/testcases/TC-test-123/artifacts/"):
			name := filepath.Base(r.URL.Path)
			putNames = append(putNames, name)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"artifact_name":"x"}`)

		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	err := runUpload(uploadFlags{
		apiBase: srv.URL,
		envFile: envPath,
		result:  resultPath,
		scripts: []string{scriptPath},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !postCalled.Load() {
		t.Error("expected POST /api/v1/testcases to be called")
	}
	if len(putNames) != 3 {
		t.Errorf("expected 3 PUT artifact calls, got %d: %v", len(putNames), putNames)
	}
}

func TestRunUpload_WithCaseID_SkipsCreate(t *testing.T) {
	dir := t.TempDir()
	envPath := writeTemp(t, dir, "env.json", `{"key":"val"}`)
	resultPath := writeTemp(t, dir, "result.txt", "data")

	var postCalled atomic.Bool
	var putCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postCalled.Store(true)
			http.Error(w, "should not be called", http.StatusInternalServerError)
			return
		}
		if r.Method == http.MethodPut {
			putCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"artifact_name":"x"}`)
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := runUpload(uploadFlags{
		apiBase: srv.URL,
		caseID:  "TC-existing-abc",
		envFile: envPath,
		result:  resultPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postCalled.Load() {
		t.Error("expected no POST call when --case-id is provided")
	}
	if putCount.Load() != 2 {
		t.Errorf("expected 2 PUT calls (env+result), got %d", putCount.Load())
	}
}

func TestRunUpload_LocalValidationFailure_NoAPICalled(t *testing.T) {
	dir := t.TempDir()
	// env.json is invalid JSON.
	envPath := writeTemp(t, dir, "env.json", `not valid json`)
	resultPath := writeTemp(t, dir, "result.txt", "data")

	var apiCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled.Store(true)
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := runUpload(uploadFlags{
		apiBase: srv.URL,
		envFile: envPath,
		result:  resultPath,
	})
	if err == nil {
		t.Error("expected error for invalid env JSON")
	}
	if apiCalled.Load() {
		t.Error("expected no API calls when local validation fails")
	}
}
