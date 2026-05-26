package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	apptestcase "github.com/AFDEAPAC/kish/internal/application/testcase"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
	"github.com/AFDEAPAC/kish/internal/domain/user"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/dto"
	"github.com/AFDEAPAC/kish/internal/interfaces/http/middleware"
)

// TestCaseHandler handles TestCase HTTP endpoints.
type TestCaseHandler struct {
	svc           *apptestcase.Service
	ownerResolver ownerDisplayNameResolver
}

type ownerDisplayNameResolver interface {
	GetUser(ctx context.Context, id string) (*user.User, error)
}

// NewTestCaseHandler constructs a TestCaseHandler.
func NewTestCaseHandler(svc *apptestcase.Service, resolvers ...ownerDisplayNameResolver) *TestCaseHandler {
	var resolver ownerDisplayNameResolver
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	return &TestCaseHandler{svc: svc, ownerResolver: resolver}
}

// CreateV1 handles POST /api/v1/testcases.
//
// Creates a TestCase metadata record only. No artifact content is accepted here;
// callers must upload files via PUT /api/v1/testcases/{case_id}/artifacts/{name}
// after receiving the case_id from this response.
// Requires authentication; the authenticated user becomes the owner.
// New TestCases are always Draft + Private regardless of any client-supplied
// status/visibility (which the request DTO does not accept).
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
		Description: req.Description,
		TestType:    req.TestType,
		Tags:        req.Tags,
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

// List handles GET /api/v1/testcases.
//
// Supported query parameters:
//   - state=all|mine|drafts|public|private (default: all)
//   - search=<substring>
//   - limit=<int>, offset=<int>
//
// Visibility filtering and ownership are enforced at the repository layer
// based on the principal: anonymous callers get only public-published, while
// developers also see their own drafts and published-private cases.
func (h *TestCaseHandler) List(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())

	q := r.URL.Query()
	selector := parseListSelector(q.Get("state"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	filter := testcase.ListFilter{
		Selector:      selector,
		CallerUserID:  p.UserID,
		CallerIsAdmin: p.Role == user.RoleAdmin,
		Search:        q.Get("search"),
		Limit:         limit,
		Offset:        offset,
	}

	cases, err := h.svc.ListTestCases(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list testcases")
		return
	}

	out := make([]dto.TestCaseResponse, 0, len(cases))
	ownerNames := h.ownerDisplayNames(r.Context(), cases)
	for _, tc := range cases {
		out = append(out, toTestCaseResponse(tc, ownerNames[tc.OwnerUserID]))
	}

	writeJSON(w, http.StatusOK, dto.ListTestCasesResponse{
		TestCases: out,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
	})
}

// GetV1 handles GET /api/v1/testcases/{case_id} with visibility-aware access.
//
// Anonymous callers see only public-published TestCases; non-public results
// return 404 to avoid leaking the existence of private/draft records.
func (h *TestCaseHandler) GetV1(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	id := r.PathValue("case_id")
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

	if !h.svc.CanRead(tc, p.UserID, p.Role == user.RoleAdmin) {
		writeError(w, http.StatusNotFound, "testcase not found: "+id)
		return
	}

	writeJSON(w, http.StatusOK, toTestCaseResponse(tc, h.ownerDisplayName(r.Context(), tc.OwnerUserID)))
}

// Get handles GET /api/testcases/{id} for backward compatibility.
//
// This endpoint is now visibility-restricted: anonymous and non-owner reads
// only succeed for public-published TestCases. Other cases return 404 to avoid
// leaking the existence of private/draft records to legacy clients.
func (h *TestCaseHandler) Get(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
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

	if !h.svc.CanRead(tc, p.UserID, p.Role == user.RoleAdmin) {
		writeError(w, http.StatusNotFound, "testcase not found: "+id)
		return
	}

	writeJSON(w, http.StatusOK, toTestCaseResponse(tc, h.ownerDisplayName(r.Context(), tc.OwnerUserID)))
}

// Update handles PATCH /api/v1/testcases/{case_id}.
//
// Requires authentication. Owner or admin may update; visibility/test_type
// changes are gated by status invariants enforced in the service layer.
func (h *TestCaseHandler) Update(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	if p.IsAnonymous {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id := r.PathValue("case_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "testcase id is required")
		return
	}

	var req dto.UpdateTestCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	patch := testcase.MetadataPatch{
		Name:        req.Name,
		Description: req.Description,
		TestType:    req.TestType,
		Tags:        req.Tags,
	}
	if req.Visibility != nil {
		v := testcase.Visibility(*req.Visibility)
		patch.Visibility = &v
	}

	tc, err := h.svc.UpdateMetadata(r.Context(), id, patch, p.UserID, p.Role == user.RoleAdmin)
	if err != nil {
		writeTestCaseError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, toTestCaseResponse(tc, h.ownerDisplayName(r.Context(), tc.OwnerUserID)))
}

// Publish handles POST /api/v1/testcases/{case_id}/publish.
//
// Body is optional; defaults to {"visibility":"public"}. Only the owner or
// an admin may publish, and only Draft TestCases can transition to Published.
func (h *TestCaseHandler) Publish(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	if p.IsAnonymous {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id := r.PathValue("case_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "testcase id is required")
		return
	}

	var req dto.PublishTestCaseRequest
	// An empty body is acceptable and means "publish as public".
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}

	tc, err := h.svc.Publish(
		r.Context(),
		id,
		testcase.Visibility(req.Visibility),
		p.UserID,
		p.Role == user.RoleAdmin,
	)
	if err != nil {
		writeTestCaseError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, toTestCaseResponse(tc, h.ownerDisplayName(r.Context(), tc.OwnerUserID)))
}

// Delete handles DELETE /api/v1/testcases/{case_id}.
func (h *TestCaseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	p := middleware.PrincipalFromContext(r.Context())
	if p.IsAnonymous {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id := r.PathValue("case_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "testcase id is required")
		return
	}

	if err := h.svc.DeleteTestCase(r.Context(), id, p.UserID, p.Role == user.RoleAdmin); err != nil {
		writeTestCaseError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// writeTestCaseError maps domain/application errors from the TestCase service
// onto appropriate HTTP status codes.
func writeTestCaseError(w http.ResponseWriter, err error, id string) {
	switch {
	case errors.Is(err, testcase.ErrNotFound):
		writeError(w, http.StatusNotFound, "testcase not found: "+id)
	case errors.Is(err, testcase.ErrForbidden):
		writeError(w, http.StatusForbidden, "you do not have permission to modify this testcase")
	case errors.Is(err, testcase.ErrNotDraft):
		writeError(w, http.StatusConflict, "this operation requires the testcase to be in draft state")
	case errors.Is(err, testcase.ErrNotPublished):
		writeError(w, http.StatusConflict, "this operation requires the testcase to be in published state")
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

// parseListSelector maps a URL query value to a domain ListSelector,
// defaulting to ListSelectorAll for empty/unknown values.
func parseListSelector(s string) testcase.ListSelector {
	switch s {
	case "mine":
		return testcase.ListSelectorMine
	case "drafts":
		return testcase.ListSelectorDrafts
	case "public":
		return testcase.ListSelectorPublic
	case "private":
		return testcase.ListSelectorPrivate
	case "", "all":
		return testcase.ListSelectorAll
	default:
		return testcase.ListSelectorAll
	}
}

func (h *TestCaseHandler) ownerDisplayName(ctx context.Context, ownerUserID string) string {
	if h.ownerResolver == nil || ownerUserID == "" {
		return ""
	}
	owner, err := h.ownerResolver.GetUser(ctx, ownerUserID)
	if err != nil || owner == nil {
		return ""
	}
	return owner.DisplayName
}

func (h *TestCaseHandler) ownerDisplayNames(ctx context.Context, cases []*testcase.TestCase) map[string]string {
	names := make(map[string]string)
	for _, tc := range cases {
		if tc == nil || tc.OwnerUserID == "" {
			continue
		}
		if _, ok := names[tc.OwnerUserID]; ok {
			continue
		}
		names[tc.OwnerUserID] = h.ownerDisplayName(ctx, tc.OwnerUserID)
	}
	return names
}

func toTestCaseResponse(tc *testcase.TestCase, ownerDisplayName string) dto.TestCaseResponse {
	scripts := make([]dto.ArtifactResponse, 0, len(tc.ScriptArtifacts))
	for _, a := range tc.ScriptArtifacts {
		scripts = append(scripts, toArtifactResponse(a))
	}
	envRefs := make([]dto.EnvironmentArtifactRefResponse, 0, len(tc.Environments))
	for _, ref := range tc.Environments {
		envRefs = append(envRefs, toEnvironmentArtifactRefResponse(ref, string(tc.DefaultEnvironmentScope)))
	}
	testScripts := make([]dto.TestScriptArtifactRefResponse, 0, len(tc.TestScripts))
	for _, ref := range tc.TestScripts {
		testScripts = append(testScripts, toTestScriptArtifactRefResponse(ref))
	}
	testResults := make([]dto.TestResultArtifactRefResponse, 0, len(tc.TestResults))
	for _, ref := range tc.TestResults {
		testResults = append(testResults, toTestResultArtifactRefResponse(ref))
	}
	return dto.TestCaseResponse{
		ID:                      tc.ID,
		Name:                    tc.Name,
		Description:             tc.Description,
		TestType:                tc.TestType,
		Tags:                    tc.Tags,
		Status:                  string(tc.Status),
		Visibility:              string(tc.Visibility),
		OwnerUserID:             tc.OwnerUserID,
		OwnerDisplayName:        ownerDisplayName,
		Environments:            envRefs,
		DefaultEnvironmentScope: string(tc.DefaultEnvironmentScope),
		TestResults:             testResults,
		TestScripts:             testScripts,
		Environment:             tc.Environment,
		ResultArtifact:          toLegacyArtifactResponse(tc.ResultArtifact),
		ScriptArtifacts:         scripts,
		CreatedAt:               tc.CreatedAt,
		UpdatedAt:               tc.UpdatedAt,
	}
}

func toEnvironmentArtifactRefResponse(ref testcase.EnvironmentArtifactRef, defaultScope string) dto.EnvironmentArtifactRefResponse {
	return dto.EnvironmentArtifactRefResponse{
		Scope:           string(ref.Scope),
		ArtifactName:    ref.ArtifactName,
		SchemaVersion:   ref.SchemaVersion,
		EnvironmentType: string(ref.EnvironmentType),
		CollectedAt:     ref.CollectedAt,
		ContentType:     ref.ContentType,
		Size:            ref.Size,
		SHA256:          ref.SHA256,
		UploadedAt:      ref.UploadedAt,
		IsDefault:       string(ref.Scope) == defaultScope,
	}
}

func toTestResultArtifactRefResponse(ref testcase.TestResultArtifactRef) dto.TestResultArtifactRefResponse {
	return dto.TestResultArtifactRefResponse{
		ArtifactName: ref.ArtifactName,
		ContentType:  ref.ContentType,
		Size:         ref.Size,
		SHA256:       ref.SHA256,
		UploadedAt:   ref.UploadedAt,
	}
}

func toTestScriptArtifactRefResponse(ref testcase.TestScriptArtifactRef) dto.TestScriptArtifactRefResponse {
	return dto.TestScriptArtifactRefResponse{
		ArtifactName: ref.ArtifactName,
		ContentType:  ref.ContentType,
		Size:         ref.Size,
		SHA256:       ref.SHA256,
		UploadedAt:   ref.UploadedAt,
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

func toLegacyArtifactResponse(a testcase.Artifact) *dto.ArtifactResponse {
	if a.Type == "" && a.Filename == "" && a.ContentType == "" && a.SizeBytes == 0 && a.SHA256 == "" && a.Content == "" && a.CreatedAt.IsZero() {
		return nil
	}
	out := toArtifactResponse(a)
	return &out
}
