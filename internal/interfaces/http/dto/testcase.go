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
type CreateTestCaseV1Request struct {
	// Name is an optional human-readable label for the test run.
	Name string `json:"name"`

	// TestType classifies the test (e.g. "sglang-benchmark", "generic").
	// Defaults to "generic" when empty.
	TestType string `json:"test_type"`
}

// CreateTestCaseResponse is returned on successful POST /api/v1/testcases.
type CreateTestCaseResponse struct {
	CaseID  string `json:"case_id"`
	Created bool   `json:"created"`
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

// TestCaseResponse is the JSON body for GET /api/testcases/{id}.
type TestCaseResponse struct {
	ID              string             `json:"id"`
	Name            string             `json:"name,omitempty"`
	TestType        string             `json:"test_type"`
	Environment     interface{}        `json:"environment,omitempty"`
	ResultArtifact  ArtifactResponse   `json:"result_artifact,omitempty"`
	ScriptArtifacts []ArtifactResponse `json:"script_artifacts,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// ErrorResponse is returned when the server encounters a client or server error.
type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthResponse is the response body for GET /healthz.
type HealthResponse struct {
	Status string `json:"status"`
}
