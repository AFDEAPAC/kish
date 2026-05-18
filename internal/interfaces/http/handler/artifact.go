package handler

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	appArtifact "github.com/AFDEAPAC/kish/internal/application/artifact"
	domArtifact "github.com/AFDEAPAC/kish/internal/domain/artifact"
	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// ArtifactHandler handles the /api/v1/testcases/{case_id}/artifacts/... endpoints.
type ArtifactHandler struct {
	svc *appArtifact.Service
}

// NewArtifactHandler constructs an ArtifactHandler.
func NewArtifactHandler(svc *appArtifact.Service) *ArtifactHandler {
	return &ArtifactHandler{svc: svc}
}

// List handles GET /api/v1/testcases/{case_id}/artifacts.
//
// Read access mirrors the parent TestCase: anonymous callers may only list
// artifacts on a public-published TestCase. Otherwise the response is 404 to
// avoid leaking the existence of private/draft cases.
func (h *ArtifactHandler) List(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("case_id")
	if caseID == "" {
		writeError(w, http.StatusBadRequest, "case_id is required")
		return
	}

	if !h.checkParentReadable(r, caseID) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("testcase %q not found", caseID))
		return
	}

	artifacts, err := h.svc.ListArtifacts(r.Context(), caseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list artifacts")
		return
	}

	items := make([]dto.ArtifactMetaResponse, 0, len(artifacts))
	for _, a := range artifacts {
		items = append(items, toArtifactMetaResponse(a))
	}

	writeJSON(w, http.StatusOK, dto.ListArtifactsResponse{
		CaseID:    caseID,
		Artifacts: items,
	})
}

// Put handles PUT /api/v1/testcases/{case_id}/artifacts/{artifact_name}.
//
// The request body is the raw file content. Artifact type is read from the
// X-Kish-Artifact-Type header; content type from the Content-Type header.
// Requires authentication; developers may only upload to their own TestCases.
func (h *ArtifactHandler) Put(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	if p.IsAnonymous {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	caseID := r.PathValue("case_id")
	artifactName := r.PathValue("artifact_name")

	if caseID == "" {
		writeError(w, http.StatusBadRequest, "case_id is required")
		return
	}

	// Enforce ownership for non-admin users: look up the TestCase owner before upload.
	if p.Role != user.RoleAdmin {
		if err := h.svc.CheckOwnership(r.Context(), caseID, p.UserID); err != nil {
			if errors.Is(err, testcase.ErrNotFound) {
				writeError(w, http.StatusNotFound, fmt.Sprintf("testcase %q not found", caseID))
				return
			}
			writeError(w, http.StatusForbidden, "you do not own this testcase")
			return
		}
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	artifactType := r.Header.Get("X-Kish-Artifact-Type")

	// Determine content length; -1 means unknown, which the local store accepts.
	size := int64(-1)
	if cl := r.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil {
			size = n
		}
	}

	a, err := h.svc.PutArtifact(r.Context(), caseID, artifactName, artifactType, contentType, r.Body, size)
	if err != nil {
		switch {
		case errors.Is(err, testcase.ErrNotFound):
			writeError(w, http.StatusNotFound, fmt.Sprintf("testcase %q not found", caseID))
		case errors.Is(err, testcase.ErrPublishedImmutable):
			writeError(w, http.StatusConflict, "result and execution-environment artifacts are immutable on published testcases")
		case isValidationError(err):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, appArtifact.ErrInsufficientStorage):
			log.Printf("[artifact] store failed case_id=%q artifact_name=%q: %v", caseID, artifactName, err)
			writeError(w, http.StatusInsufficientStorage, "artifact storage is full")
		default:
			log.Printf("[artifact] store failed case_id=%q artifact_name=%q: %v", caseID, artifactName, err)
			writeError(w, http.StatusInternalServerError, "failed to store artifact")
		}
		return
	}

	writeJSON(w, http.StatusOK, toArtifactMetaResponse(a))
}

// Get handles GET /api/v1/testcases/{case_id}/artifacts/{artifact_name}.
// The artifact content is streamed directly to the response body.
//
// Visibility is enforced against the parent TestCase: anonymous users only see
// content of public-published TestCases. Otherwise the response is 404.
func (h *ArtifactHandler) Get(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("case_id")
	artifactName := r.PathValue("artifact_name")

	if caseID == "" {
		writeError(w, http.StatusBadRequest, "case_id is required")
		return
	}

	if !h.checkParentReadable(r, caseID) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("testcase %q not found", caseID))
		return
	}

	meta, rc, err := h.svc.GetArtifact(r.Context(), caseID, artifactName)
	if err != nil {
		switch {
		case errors.Is(err, domArtifact.ErrNotFound):
			writeError(w, http.StatusNotFound, fmt.Sprintf("artifact %q not found", artifactName))
		case isValidationError(err):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			log.Printf("[artifact] retrieve failed case_id=%q artifact_name=%q: %v", caseID, artifactName, err)
			writeError(w, http.StatusInternalServerError, "failed to retrieve artifact")
		}
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", meta.ContentType)
	if meta.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, meta.ArtifactName))
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, rc); err != nil {
		// Headers are already sent; log the error but do not write another response.
		_ = err
	}
}

// Delete handles DELETE /api/v1/testcases/{case_id}/artifacts/{artifact_name}.
// Requires authentication; developers may only delete artifacts from their own TestCases.
func (h *ArtifactHandler) Delete(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	if p.IsAnonymous {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	caseID := r.PathValue("case_id")
	artifactName := r.PathValue("artifact_name")

	if caseID == "" {
		writeError(w, http.StatusBadRequest, "case_id is required")
		return
	}

	// Enforce ownership for non-admin users.
	if p.Role != user.RoleAdmin {
		if err := h.svc.CheckOwnership(r.Context(), caseID, p.UserID); err != nil {
			if errors.Is(err, testcase.ErrNotFound) {
				writeError(w, http.StatusNotFound, fmt.Sprintf("testcase %q not found", caseID))
				return
			}
			writeError(w, http.StatusForbidden, "you do not own this testcase")
			return
		}
	}

	if err := h.svc.DeleteArtifact(r.Context(), caseID, artifactName); err != nil {
		switch {
		case errors.Is(err, testcase.ErrPublishedImmutable):
			writeError(w, http.StatusConflict, "result and execution-environment artifacts are immutable on published testcases")
		case isValidationError(err):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			log.Printf("[artifact] delete failed case_id=%q artifact_name=%q: %v", caseID, artifactName, err)
			writeError(w, http.StatusInternalServerError, "failed to delete artifact")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// checkParentReadable reports whether the requesting principal may see the
// parent TestCase. Used by anonymous-friendly read endpoints (List, Get) so
// they cannot leak existence of private or draft cases.
//
// Returns true when the TestCase exists and the caller satisfies one of:
//
//	admin role, or
//	owner of the TestCase, or
//	parent TestCase is public-published.
//
// Returns false when the TestCase is missing or the caller is not allowed to
// see it; callers should respond with 404 in either case.
func (h *ArtifactHandler) checkParentReadable(r *http.Request, caseID string) bool {
	tc, err := h.svc.FindTestCase(r.Context(), caseID)
	if err != nil {
		return false
	}
	p := middleware.PrincipalFromContext(r.Context())
	if p.Role == user.RoleAdmin {
		return true
	}
	if tc.IsPubliclyReadable() {
		return true
	}
	if p.UserID != "" && tc.OwnerUserID != "" && tc.OwnerUserID == p.UserID {
		return true
	}
	return false
}

// toArtifactMetaResponse maps a domain Artifact to its API response DTO.
// StorageKey is never included in the response.
func toArtifactMetaResponse(a *domArtifact.Artifact) dto.ArtifactMetaResponse {
	return dto.ArtifactMetaResponse{
		CaseID:       a.CaseID,
		ArtifactName: a.ArtifactName,
		ArtifactType: string(a.ArtifactType),
		ContentType:  a.ContentType,
		Size:         a.Size,
		SHA256:       a.SHA256,
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
	}
}

// isValidationError reports whether err is a client-caused validation error.
// Validation errors come from artifact.ValidateArtifactName or invalid type checks,
// which return plain errors with descriptive messages (not sentinel values).
// This heuristic keeps the service errors usable as 400 responses.
func isValidationError(err error) bool {
	if errors.Is(err, domArtifact.ErrNotFound) || errors.Is(err, testcase.ErrNotFound) {
		return false
	}
	if errors.Is(err, environment.ErrInvalidSnapshot) {
		return true
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "artifact_name ") || strings.HasPrefix(msg, "invalid artifact_type ")
}
