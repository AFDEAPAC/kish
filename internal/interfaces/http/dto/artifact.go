package dto

import "time"

// ArtifactMetaResponse is the JSON representation of an artifact's metadata.
// StorageKey is intentionally omitted; internal paths must not be exposed via API.
type ArtifactMetaResponse struct {
	CaseID       string    `json:"case_id"`
	ArtifactName string    `json:"artifact_name"`
	ArtifactType string    `json:"artifact_type"`
	ContentType  string    `json:"content_type"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"checksum_sha256"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ListArtifactsResponse is returned by GET /api/v1/testcases/{case_id}/artifacts.
type ListArtifactsResponse struct {
	CaseID    string                 `json:"case_id"`
	Artifacts []ArtifactMetaResponse `json:"artifacts"`
}
