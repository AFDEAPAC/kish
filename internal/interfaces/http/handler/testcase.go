package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	apptestcase "github.com/AFDEAPAC/kish/internal/application/testcase"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// TestCaseHandler handles TestCase HTTP endpoints.
type TestCaseHandler struct {
	svc *apptestcase.Service
}

// NewTestCaseHandler constructs a TestCaseHandler.
func NewTestCaseHandler(svc *apptestcase.Service) *TestCaseHandler {
	return &TestCaseHandler{svc: svc}
}

// CreateV1 handles POST /api/v1/testcases.
//
// Creates a TestCase metadata record only. No artifact content is accepted here;
// callers must upload files via PUT /api/v1/testcases/{case_id}/artifacts/{name}
// after receiving the case_id from this response.
// Requires authentication; the authenticated user becomes the owner.
func (h *TestCaseHandler) CreateV1(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	if p.IsAnonymous {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req dto.CreateTestCaseV1Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	tc, err := h.svc.CreateMetadata(r.Context(), testcase.MetadataInput{
		Name:        req.Name,
		TestType:    req.TestType,
		OwnerUserID: p.UserID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create testcase: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, dto.CreateTestCaseResponse{
		CaseID:  tc.ID,
		Created: true,
	})
}

// Get handles GET /api/testcases/{id}.
func (h *TestCaseHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "testcase id is required")
		return
	}

	tc, err := h.svc.GetTestCase(r.Context(), id)
	if err != nil {
		if errors.Is(err, testcase.ErrNotFound) {
			writeError(w, http.StatusNotFound, "testcase not found: "+id)
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to retrieve testcase")
		return
	}

	writeJSON(w, http.StatusOK, toTestCaseResponse(tc))
}

func toTestCaseResponse(tc *testcase.TestCase) dto.TestCaseResponse {
	scripts := make([]dto.ArtifactResponse, 0, len(tc.ScriptArtifacts))
	for _, a := range tc.ScriptArtifacts {
		scripts = append(scripts, toArtifactResponse(a))
	}
	return dto.TestCaseResponse{
		ID:              tc.ID,
		Name:            tc.Name,
		TestType:        tc.TestType,
		Environment:     tc.Environment,
		ResultArtifact:  toArtifactResponse(tc.ResultArtifact),
		ScriptArtifacts: scripts,
		CreatedAt:       tc.CreatedAt,
		UpdatedAt:       tc.UpdatedAt,
	}
}

func toArtifactResponse(a testcase.Artifact) dto.ArtifactResponse {
	return dto.ArtifactResponse{
		Type:        string(a.Type),
		Filename:    a.Filename,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		SHA256:      a.SHA256,
		Content:     a.Content,
		CreatedAt:   a.CreatedAt,
	}
}
