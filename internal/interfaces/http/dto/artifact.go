package dto

import "time"

// ArtifactMetaResponse is the public-safe projection of a domain Artifact.
// StorageKey (the bucket / filesystem path) is intentionally excluded so
// internal layout details cannot be inferred or used to bypass the
// authorization checks the API performs before returning content. SHA256
// is exposed under the JSON name "checksum_sha256" to make it discoverable
// without binding API consumers to a specific algorithm name.
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

// ListArtifactsResponse is the outbound payload of
// GET /api/v1/testcases/{case_id}/artifacts. Visibility filtering has
// already been performed by the handler against the parent TestCase; an
// authorised caller sees every artifact attached to the case in the order
// returned by the repository (currently insertion order, not stable across
// repository implementations).
type ListArtifactsResponse struct {
	CaseID    string                 `json:"case_id"`
	Artifacts []ArtifactMetaResponse `json:"artifacts"`
}
