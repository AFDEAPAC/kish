package mongodb

import (
	"testing"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/domain/testcase"
)

func TestFromDocumentPrefersCanonicalEnvironmentRefs(t *testing.T) {
	collectedAt := time.Date(2026, 5, 6, 13, 24, 15, 0, time.UTC)
	tc, err := fromDocument(testCaseDocument{
		ID:              "TC-1",
		TestType:        "generic",
		Status:          string(testcase.StatusDraft),
		Visibility:      string(testcase.VisibilityPrivate),
		EnvironmentJSON: `{"schema_version":"environment-snapshot/v1","scope":"execution","type":"host","collected_at":"2026-05-06T13:24:15Z","data":{}}`,
		Environments: []environmentRefDocument{{
			Scope:           "execution",
			ArtifactName:    "env.json",
			SchemaVersion:   environment.SchemaVersionV1,
			EnvironmentType: string(environment.TypeContainer),
			CollectedAt:     collectedAt,
		}},
		CreatedAt: collectedAt,
		UpdatedAt: collectedAt,
	})
	if err != nil {
		t.Fatalf("fromDocument: %v", err)
	}
	if len(tc.Environments) != 1 {
		t.Fatalf("expected canonical environment ref, got %d", len(tc.Environments))
	}
	if tc.Environments[0].ArtifactName != "env.json" || tc.Environments[0].EnvironmentType != environment.TypeContainer {
		t.Fatalf("expected canonical env ref to be preserved, got %#v", tc.Environments[0])
	}
	if tc.DefaultEnvironmentScope != environment.ScopeExecution {
		t.Fatalf("expected default execution scope, got %q", tc.DefaultEnvironmentScope)
	}
}

func TestFromDocumentLegacyEnvironmentDoesNotInventCanonicalRef(t *testing.T) {
	tc, err := fromDocument(testCaseDocument{
		ID:              "TC-legacy",
		TestType:        "generic",
		Status:          string(testcase.StatusDraft),
		Visibility:      string(testcase.VisibilityPrivate),
		EnvironmentJSON: `{"schema_version":"environment-snapshot/v1","scope":"execution","type":"container","collected_at":"2026-05-06T13:24:15Z","data":{}}`,
	})
	if err != nil {
		t.Fatalf("fromDocument: %v", err)
	}
	if len(tc.Environments) != 0 {
		t.Fatalf("expected no canonical refs invented from legacy environment_json, got %#v", tc.Environments)
	}
	if tc.Environment == nil {
		t.Fatalf("expected legacy environment to remain readable")
	}
}

func TestToDocumentOmitsLegacyArtifactSlotsForNewWrites(t *testing.T) {
	doc, err := toDocument(&testcase.TestCase{
		ID:         "TC-new",
		TestType:   "generic",
		Status:     testcase.StatusDraft,
		Visibility: testcase.VisibilityPrivate,
		Environments: []testcase.EnvironmentArtifactRef{{
			Scope:        environment.ScopeExecution,
			ArtifactName: "env.json",
		}},
		TestResults: []testcase.TestResultArtifactRef{{ArtifactName: "result.txt"}},
		TestScripts: []testcase.TestScriptArtifactRef{{
			ArtifactName: "run.sh",
		}},
	})
	if err != nil {
		t.Fatalf("toDocument: %v", err)
	}
	if doc.EnvironmentJSON != "" {
		t.Fatalf("expected new document not to write environment_json, got %q", doc.EnvironmentJSON)
	}
	if doc.ResultArtifact.Filename != "" || len(doc.ScriptArtifacts) != 0 {
		t.Fatalf("expected new document not to write legacy artifacts, got result=%#v scripts=%#v", doc.ResultArtifact, doc.ScriptArtifacts)
	}
	if doc.TestResult != nil {
		t.Fatalf("expected new document not to write legacy test_result, got %#v", doc.TestResult)
	}
	if len(doc.TestResults) != 1 || doc.TestResults[0].ArtifactName != "result.txt" {
		t.Fatalf("expected test_results to contain result.txt, got %#v", doc.TestResults)
	}
}

func TestFromDocumentReadsLegacyTestResultAsResults(t *testing.T) {
	tc, err := fromDocument(testCaseDocument{
		ID:         "TC-legacy-result",
		TestType:   "generic",
		Status:     string(testcase.StatusDraft),
		Visibility: string(testcase.VisibilityPrivate),
		TestResult: &artifactRefDocument{
			ArtifactName: "result.txt",
			ContentType:  "text/plain",
			Size:         12,
			SHA256:       "abc123",
		},
	})
	if err != nil {
		t.Fatalf("fromDocument: %v", err)
	}
	if len(tc.TestResults) != 1 {
		t.Fatalf("expected one migrated result ref, got %#v", tc.TestResults)
	}
	if tc.TestResults[0].ArtifactName != "result.txt" || tc.TestResults[0].SHA256 != "abc123" {
		t.Fatalf("expected legacy test_result to migrate, got %#v", tc.TestResults[0])
	}
}
