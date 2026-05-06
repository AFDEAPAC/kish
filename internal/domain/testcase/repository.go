package testcase

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Repository implementations when the requested
// TestCase does not exist in the backing store.
var ErrNotFound = errors.New("testcase not found")

// ErrNotDraft is returned when an operation requires a draft TestCase but
// the target is not in the draft state (e.g. publishing an already-published
// TestCase, or modifying test_type after publish).
var ErrNotDraft = errors.New("testcase is not in draft state")

// ErrNotPublished is returned when an operation requires a published TestCase
// but the target is still in draft state (e.g. changing visibility).
var ErrNotPublished = errors.New("testcase is not in published state")

// ErrPublishedImmutable is returned when an operation attempts to mutate an
// artifact (result or execution environment) that is part of a published TestCase.
var ErrPublishedImmutable = errors.New("artifact is immutable on published testcase")

// ErrForbidden is returned when the caller does not have permission to
// perform the requested operation on the TestCase.
var ErrForbidden = errors.New("forbidden")

// ListSelector identifies which TestCases the caller wants to see.
// It is interpreted by the repository against the requesting principal.
type ListSelector string

const (
	// ListSelectorAll requests every TestCase the caller is allowed to see.
	// For anonymous callers this is equivalent to ListSelectorPublic.
	// For authenticated developers this is "public-published + own".
	// For admins this is every TestCase.
	ListSelectorAll ListSelector = "all"

	// ListSelectorMine requests TestCases owned by the requesting user.
	// Not meaningful for anonymous callers.
	ListSelectorMine ListSelector = "mine"

	// ListSelectorDrafts requests draft TestCases owned by the requesting user
	// (or all drafts when caller is admin).
	ListSelectorDrafts ListSelector = "drafts"

	// ListSelectorPublic requests public-published TestCases. Always available.
	ListSelectorPublic ListSelector = "public"

	// ListSelectorPrivate requests published-private TestCases the caller can see.
	// Anonymous callers receive no results.
	ListSelectorPrivate ListSelector = "private"
)

// ListFilter parameterises Repository.List.
//
// Visibility/status filtering is computed by the repository from Selector,
// CallerUserID, and CallerIsAdmin. The repository must never return TestCases
// that the caller is not allowed to see.
type ListFilter struct {
	// Selector selects the high-level TestCase category.
	Selector ListSelector

	// CallerUserID is the authenticated user's ID, or empty for anonymous.
	CallerUserID string

	// CallerIsAdmin is true when the caller has the admin role.
	CallerIsAdmin bool

	// Search, when non-empty, filters by case-insensitive substring match
	// against the TestCase ID and Name.
	Search string

	// Limit caps the number of results. Repository implementations should
	// apply a sensible default and maximum when Limit is zero or excessive.
	Limit int

	// Offset is the number of records to skip. Used together with Limit
	// for simple offset-based pagination.
	Offset int
}

// Repository is the persistence interface for TestCase entities.
//
// Implementations live in the infrastructure layer (e.g. infrastructure/mongodb).
// The domain and application layers depend only on this interface, keeping them
// free of any database-specific code.
type Repository interface {
	// Create persists a new TestCase. The TestCase ID must already be set by
	// the caller (application layer generates it before calling Create).
	Create(ctx context.Context, tc *TestCase) error

	// Update replaces the environment, artifacts, and metadata of an existing
	// TestCase identified by id. CreatedAt must be preserved by the implementation.
	// Returns ErrNotFound if the id does not exist.
	Update(ctx context.Context, id string, tc *TestCase) error

	// UpdatePartial applies a partial metadata patch to the TestCase identified by id.
	// Implementations must set updated_at to the current time. Status is never
	// modified through this method; use UpdateStatus for the publish workflow.
	// Returns ErrNotFound if the id does not exist.
	UpdatePartial(ctx context.Context, id string, patch MetadataPatch) error

	// UpdateStatus transitions the TestCase to a new status and visibility.
	// This method is used by the Publish workflow. Implementations must update
	// updated_at and persist both fields atomically.
	// Returns ErrNotFound if the id does not exist.
	UpdateStatus(ctx context.Context, id string, status Status, visibility Visibility) error

	// FindByID retrieves a TestCase by its ID.
	// Returns ErrNotFound if the id does not exist.
	FindByID(ctx context.Context, id string) (*TestCase, error)

	// List returns TestCases matching the given filter. The repository is
	// responsible for translating Selector + caller identity into concrete
	// status/visibility/owner predicates so authorization is enforced at the
	// data layer.
	List(ctx context.Context, filter ListFilter) ([]*TestCase, error)

	// Delete removes the TestCase identified by id.
	// Returns ErrNotFound if the id does not exist.
	Delete(ctx context.Context, id string) error
}
