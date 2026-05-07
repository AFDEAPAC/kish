// Package dto defines the HTTP request and response types for the kish API.
//
// DTOs are intentionally separated from domain types so that changes to the
// HTTP API contract do not require changes to the domain model, and vice versa.
package dto

import (
	"time"
)

// CreateTestCaseV1Request is the JSON body for POST /api/v1/testcases.
// Only metadata is accepted at creation time; file content is uploaded separately
// via the artifact API (PUT /api/v1/testcases/{case_id}/artifacts/{name}).
//
// New TestCases are always created as Draft + Private; status and visibility
// cannot be supplied here.
type CreateTestCaseV1Request struct {
	// Name is an optional human-readable label for the test run.
	Name string `json:"name"`

	// Description is an optional long-form description.
	Description string `json:"description"`

	// TestType classifies the test (e.g. "sglang-benchmark", "generic").
	// Defaults to "generic" when empty.
	TestType string `json:"test_type"`

	// Tags are optional user-defined labels.
	Tags []string `json:"tags"`
}

// CreateTestCaseResponse is returned on successful POST /api/v1/testcases.
type CreateTestCaseResponse struct {
	CaseID  string `json:"case_id"`
	Created bool   `json:"created"`
}

// UpdateTestCaseRequest is the JSON body for PATCH /api/v1/testcases/{case_id}.
//
// Pointer fields encode "absent" vs "explicitly set" semantics. JSON null is
// not accepted; clients must omit a field they do not want to change.
//
// Field-level rules enforced by the application service:
//   - test_type may only change while status=draft
//   - visibility may only change while status=published
type UpdateTestCaseRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	TestType    *string   `json:"test_type,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	Visibility  *string   `json:"visibility,omitempty"`
}

// PublishTestCaseRequest is the JSON body for POST /api/v1/testcases/{case_id}/publish.
type PublishTestCaseRequest struct {
	// Visibility is the chosen visibility for the published TestCase.
	// Optional; defaults to "public" when absent or empty.
	Visibility string `json:"visibility,omitempty"`
}

// ArtifactResponse is the JSON representation of an inline artifact in a GET response.
// Present only on TestCase documents created before the artifact API was introduced.
type ArtifactResponse struct {
	Type        string    `json:"type"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	SHA256      string    `json:"sha256"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

// EnvironmentArtifactRefResponse is the TestCase-level summary of an uploaded
// EnvironmentSnapshot artifact. The raw JSON remains downloadable via the
// Artifact API.
type EnvironmentArtifactRefResponse struct {
	Scope           string    `json:"scope"`
	ArtifactName    string    `json:"artifact_name"`
	SchemaVersion   string    `json:"schema_version,omitempty"`
	EnvironmentType string    `json:"environment_type,omitempty"`
	CollectedAt     time.Time `json:"collected_at,omitempty"`
	ContentType     string    `json:"content_type,omitempty"`
	Size            int64     `json:"size,omitempty"`
	SHA256          string    `json:"checksum_sha256,omitempty"`
	UploadedAt      time.Time `json:"uploaded_at,omitempty"`
	IsDefault       bool      `json:"is_default"`
}

// TestResultArtifactRefResponse points to the current result artifact.
type TestResultArtifactRefResponse struct {
	ArtifactName string    `json:"artifact_name"`
	ContentType  string    `json:"content_type,omitempty"`
	Size         int64     `json:"size,omitempty"`
	SHA256       string    `json:"checksum_sha256,omitempty"`
	UploadedAt   time.Time `json:"uploaded_at,omitempty"`
}

// TestScriptArtifactRefResponse points to a script artifact associated with a TestCase.
type TestScriptArtifactRefResponse struct {
	ArtifactName string    `json:"artifact_name"`
	ContentType  string    `json:"content_type,omitempty"`
	Size         int64     `json:"size,omitempty"`
	SHA256       string    `json:"checksum_sha256,omitempty"`
	UploadedAt   time.Time `json:"uploaded_at,omitempty"`
}

// TestCaseResponse is the JSON body for GET /api/testcases/{id} and
// GET /api/v1/testcases/{case_id}.
type TestCaseResponse struct {
	ID                      string                           `json:"id"`
	Name                    string                           `json:"name,omitempty"`
	Description             string                           `json:"description,omitempty"`
	TestType                string                           `json:"test_type"`
	Tags                    []string                         `json:"tags,omitempty"`
	Status                  string                           `json:"status"`
	Visibility              string                           `json:"visibility"`
	OwnerUserID             string                           `json:"owner_user_id,omitempty"`
	OwnerDisplayName        string                           `json:"owner_display_name,omitempty"`
	Environments            []EnvironmentArtifactRefResponse `json:"environments"`
	DefaultEnvironmentScope string                           `json:"default_environment_scope,omitempty"`
	TestResult              *TestResultArtifactRefResponse   `json:"test_result,omitempty"`
	TestScripts             []TestScriptArtifactRefResponse  `json:"test_scripts"`
	Environment             interface{}                      `json:"environment,omitempty"`
	ResultArtifact          *ArtifactResponse                `json:"result_artifact,omitempty"`
	ScriptArtifacts         []ArtifactResponse               `json:"script_artifacts,omitempty"`
	CreatedAt               time.Time                        `json:"created_at"`
	UpdatedAt               time.Time                        `json:"updated_at"`
}

// ListTestCasesResponse is the JSON body for GET /api/v1/testcases.
//
// Pagination is offset-based for v1. Total counts are intentionally omitted
// to keep the implementation simple; clients can detect end-of-list when
// len(testcases) < limit.
type ListTestCasesResponse struct {
	TestCases []TestCaseResponse `json:"testcases"`
	Limit     int                `json:"limit"`
	Offset    int                `json:"offset"`
}

// ErrorResponse is returned when the server encounters a client or server error.
type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthResponse is the response body for GET /healthz.
type HealthResponse struct {
	Status string `json:"status"`
}
