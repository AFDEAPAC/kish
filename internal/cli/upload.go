package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
)

type uploadFlags struct {
	apiBase    string
	envFile    string
	result     string
	scripts    []string
	rawFiles   []string
	logFiles   []string
	otherFiles []string
	caseID     string
	name       string
	testType   string
	token      string // --token flag; KISH_API_TOKEN env var is checked as fallback
}

// uploadArtifact represents a single file to be uploaded to the artifact API.
type uploadArtifact struct {
	localPath    string
	artifactName string // filepath.Base(localPath)
	artifactType string // "environment" | "result" | "script" | "raw" | "log" | "other"
	contentType  string // optional explicit MIME type; inferred from artifactName when empty
	content      string
}

const (
	maxUploadEnvironmentBytes = 5 * 1024 * 1024
	maxUploadResultBytes      = 5 * 1024 * 1024
	maxUploadScriptBytes      = 1 * 1024 * 1024
	maxUploadAuxiliaryBytes   = 5 * 1024 * 1024
)

// newUploadCmd constructs the `kish upload` cobra command.
func newUploadCmd() *cobra.Command {
	var flags uploadFlags

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload files to a kish TestCase via the artifact API",
		Long: `upload validates local files, ensures a TestCase exists, then uploads
each file as a named artifact via PUT /api/v1/testcases/{case_id}/artifacts/{name}.

Without --case-id, a new TestCase metadata record is created first and the
generated case_id is printed before artifact uploads begin.

With --case-id, the provided case_id is used directly. The artifact API returns
404 if the case_id does not exist; no pre-flight existence check is performed.

Authentication:
  Provide a client token via --token or the KISH_API_TOKEN environment variable.
  --token takes priority over the environment variable.
  Provide the API base URL via --api or the KISH_API_URL environment variable.
  --api takes priority over the environment variable.
  When the server requires authentication and no token is provided, a clear error
  is printed and the command exits non-zero.

Examples:
  kish upload --api http://127.0.0.1:30151 --token kish_xxx --env env.json --result result.txt
  export KISH_API_TOKEN=kish_xxx
  export KISH_API_URL=http://127.0.0.1:30151
  kish upload --env env.json --result result.txt --script run.sh \
              --log server.log --raw metrics.json --other notes.txt \
              --name "sglang test" --type sglang-benchmark
  kish upload --api http://127.0.0.1:30151 \
              --case-id TC-20260505143022-a8f3 \
              --env env.json --result result.txt --script run.sh --log worker.log`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpload(flags)
		},
	}

	cmd.Flags().StringVar(&flags.apiBase, "api", "", "Base URL of the kish API server (overrides KISH_API_URL)")
	cmd.Flags().StringVar(&flags.envFile, "env", "", "Path to environment snapshot JSON (required)")
	cmd.Flags().StringVar(&flags.result, "result", "", "Path to test result text file (required)")
	cmd.Flags().StringArrayVar(&flags.scripts, "script", nil, "Path to a test script (repeatable)")
	cmd.Flags().StringArrayVar(&flags.rawFiles, "raw", nil, "Path to a raw artifact file (repeatable)")
	cmd.Flags().StringArrayVar(&flags.logFiles, "log", nil, "Path to a log artifact file (repeatable)")
	cmd.Flags().StringArrayVar(&flags.otherFiles, "other", nil, "Path to an uncategorized artifact file (repeatable)")
	cmd.Flags().StringVar(&flags.caseID, "case-id", "", "Existing TestCase ID; when set, artifacts are uploaded to this case")
	cmd.Flags().StringVar(&flags.name, "name", "", "Human-readable name for a newly created TestCase")
	cmd.Flags().StringVar(&flags.testType, "type", "generic", "Test type for a newly created TestCase (e.g. sglang-benchmark)")
	cmd.Flags().StringVar(&flags.token, "token", "", "Client token for API authentication (overrides KISH_API_TOKEN)")

	_ = cmd.MarkFlagRequired("env")
	_ = cmd.MarkFlagRequired("result")

	return cmd
}

// resolveToken returns the API token to use, following the priority order:
// 1. --token flag
// 2. KISH_API_TOKEN environment variable
// Returns empty string if neither is set.
func resolveToken(flags uploadFlags) string {
	return resolveTokenValue(flags.token)
}

// resolveAPIBase returns the API base URL to use, following the priority order:
// 1. --api flag
// 2. KISH_API_URL environment variable
// Returns empty string if neither is set.
func resolveAPIBase(flags uploadFlags) string {
	return resolveAPIBaseValue(flags.apiBase)
}

// runUpload is the top-level upload workflow.
//
// Order of operations:
//  1. Collect and validate all local files (no API calls yet).
//  2. Resolve case_id (create metadata if --case-id is absent).
//  3. Upload all artifacts via the unified PUT endpoint.
func runUpload(flags uploadFlags) error {
	token := resolveToken(flags)
	apiBase := resolveAPIBase(flags)
	if apiBase == "" {
		return fmt.Errorf("API base URL is required: provide --api or set KISH_API_URL")
	}
	flags.apiBase = apiBase

	artifacts, err := buildUploadArtifacts(flags)
	if err != nil {
		return err
	}

	caseID, created, err := ensureCaseID(flags, token)
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("created testcase: %s\n", caseID)
	}

	return uploadArtifacts(caseID, artifacts, flags.apiBase, token)
}

// buildUploadArtifacts reads and validates all local files, returning a list of
// artifacts ready for upload. Validation failures are returned before any API call.
func buildUploadArtifacts(flags uploadFlags) ([]uploadArtifact, error) {
	var artifacts []uploadArtifact

	// Environment snapshot: required, must be valid JSON.
	envContent, err := readTextFile(flags.envFile, maxUploadEnvironmentBytes)
	if err != nil {
		return nil, fmt.Errorf("--env: %w", err)
	}
	if !json.Valid([]byte(envContent)) {
		return nil, fmt.Errorf("--env: file is not valid JSON: %s", flags.envFile)
	}
	artifacts = append(artifacts, uploadArtifact{
		localPath:    flags.envFile,
		artifactName: filepath.Base(flags.envFile),
		artifactType: "environment",
		content:      envContent,
	})

	// Test result: required and intentionally singular. Additional result-like
	// files should be uploaded as raw/log/other artifacts.
	resultContent, err := readTextFile(flags.result, maxUploadResultBytes)
	if err != nil {
		return nil, fmt.Errorf("--result: %w", err)
	}
	artifacts = append(artifacts, uploadArtifact{
		localPath:    flags.result,
		artifactName: filepath.Base(flags.result),
		artifactType: "result",
		content:      resultContent,
	})

	var typedErr error
	artifacts, typedErr = appendTypedArtifacts(artifacts, flags.scripts, "script", maxUploadScriptBytes, "--script")
	if typedErr != nil {
		return nil, typedErr
	}
	artifacts, typedErr = appendTypedArtifacts(artifacts, flags.rawFiles, "raw", maxUploadAuxiliaryBytes, "--raw")
	if typedErr != nil {
		return nil, typedErr
	}
	artifacts, typedErr = appendTypedArtifacts(artifacts, flags.logFiles, "log", maxUploadAuxiliaryBytes, "--log")
	if typedErr != nil {
		return nil, typedErr
	}
	artifacts, typedErr = appendTypedArtifacts(artifacts, flags.otherFiles, "other", maxUploadAuxiliaryBytes, "--other")
	if typedErr != nil {
		return nil, typedErr
	}

	return artifacts, nil
}

// appendTypedArtifacts converts repeatable typed flags into artifact upload
// records while keeping validation errors tied to the originating flag. The
// caller supplies the API artifact type because script/raw/log/other share the
// same local-file workflow but have different server-side semantics.
func appendTypedArtifacts(artifacts []uploadArtifact, paths []string, artifactType string, maxBytes int64, flagName string) ([]uploadArtifact, error) {
	for _, path := range paths {
		content, err := readTextFile(path, maxBytes)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", flagName, path, err)
		}
		artifacts = append(artifacts, uploadArtifact{
			localPath:    path,
			artifactName: filepath.Base(path),
			artifactType: artifactType,
			content:      content,
		})
	}
	return artifacts, nil
}

// ensureCaseID returns the case_id to use for artifact uploads.
// If --case-id is provided it is returned directly. Otherwise a new TestCase
// metadata record is created via POST /api/v1/testcases and the new ID is returned.
// The boolean return value is true only when a new TestCase was created.
func ensureCaseID(flags uploadFlags, token string) (caseID string, created bool, err error) {
	if flags.caseID != "" {
		return flags.caseID, false, nil
	}

	apiBase := strings.TrimSuffix(flags.apiBase, "/")
	url := apiBase + "/api/v1/testcases"

	reqBody := dto.CreateTestCaseV1Request{
		Name:     flags.name,
		TestType: flags.testType,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", false, fmt.Errorf("failed to encode create request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", false, fmt.Errorf("build create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("create testcase: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusCreated:
		// success, fall through
	case http.StatusUnauthorized:
		return "", false, fmt.Errorf("authentication required: provide --token or set KISH_API_TOKEN")
	case http.StatusForbidden:
		return "", false, fmt.Errorf("permission denied: your token does not have testcase:write scope")
	default:
		return "", false, fmt.Errorf("create testcase: server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var out dto.CreateTestCaseResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", false, fmt.Errorf("create testcase: failed to decode response: %w", err)
	}
	return out.CaseID, true, nil
}

// uploadArtifacts uploads each artifact to PUT /api/v1/testcases/{caseID}/artifacts/{name}.
// All uploads share the same case_id regardless of whether it was newly created or provided.
func uploadArtifacts(caseID string, artifacts []uploadArtifact, apiBase, token string) error {
	for _, a := range artifacts {
		if err := uploadArtifactContent(caseID, a, apiBase, token); err != nil {
			return err
		}
		fmt.Printf("uploaded artifact: %s (type=%s)\n", a.artifactName, a.artifactType)
	}

	fmt.Printf("artifacts uploaded to testcase: %s\n", caseID)
	return nil
}

// uploadEnvironmentSnapshot uploads an in-memory EnvironmentSnapshot JSON
// without requiring the caller to write a temporary file first.
func uploadEnvironmentSnapshot(caseID, artifactName string, content []byte, apiBase, token string) error {
	return uploadArtifactContent(caseID, uploadArtifact{
		artifactName: artifactName,
		artifactType: "environment",
		contentType:  "application/json",
		content:      string(content),
	}, apiBase, token)
}

// uploadArtifactContent uploads one artifact body to the unified Artifact API.
// CLI commands own user-facing progress output so this helper stays quiet.
func uploadArtifactContent(caseID string, a uploadArtifact, apiBase, token string) error {
	base := strings.TrimSuffix(apiBase, "/")
	url := fmt.Sprintf("%s/api/v1/testcases/%s/artifacts/%s", base, caseID, a.artifactName)
	contentType := a.contentType
	if contentType == "" {
		contentType = contentTypeForFile(a.artifactName)
	}

	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(a.content))
	if err != nil {
		return fmt.Errorf("build request for %q: %w", a.artifactName, err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Kish-Artifact-Type", a.artifactType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload %q: %w", a.artifactName, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("upload %q: authentication required — provide --token or set KISH_API_TOKEN", a.artifactName)
	case http.StatusForbidden:
		return fmt.Errorf("upload %q: permission denied — your token does not have testcase:write scope", a.artifactName)
	default:
		return fmt.Errorf("upload %q: server returned %d: %s", a.artifactName, resp.StatusCode, string(body))
	}
}

// readTextFile reads a file and returns its content as a string.
// Returns an error if the file does not exist, is not readable, or exceeds maxBytes.
func readTextFile(path string, maxBytes int64) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", path)
		}
		return "", fmt.Errorf("cannot access %s: %w", path, err)
	}
	if info.Size() > maxBytes {
		return "", fmt.Errorf("file %s exceeds maximum size of %d bytes", path, maxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	return string(data), nil
}

// contentTypeForFile returns a best-effort content type based on file extension.
func contentTypeForFile(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		return "application/json"
	case ".txt", ".log":
		return "text/plain"
	case ".sh":
		return "text/x-shellscript"
	case ".yaml", ".yml":
		return "application/yaml"
	default:
		return "application/octet-stream"
	}
}
