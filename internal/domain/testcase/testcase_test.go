package testcase

import (
	"testing"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
)

func TestSetEnvironmentArtifact_SetsExecutionDefault(t *testing.T) {
	tc := &TestCase{}
	tc.SetEnvironmentArtifact(EnvironmentArtifactRef{
		Scope:        environment.ScopeExecution,
		ArtifactName: "env.json",
		UploadedAt:   time.Now(),
	})

	if len(tc.Environments) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(tc.Environments))
	}
	if tc.DefaultEnvironmentScope != environment.ScopeExecution {
		t.Fatalf("expected default execution, got %q", tc.DefaultEnvironmentScope)
	}
}

func TestSetEnvironmentArtifact_ReplacesByScope(t *testing.T) {
	tc := &TestCase{}
	tc.SetEnvironmentArtifact(EnvironmentArtifactRef{Scope: environment.ScopeExecution, ArtifactName: "env-v1.json"})
	tc.SetEnvironmentArtifact(EnvironmentArtifactRef{Scope: environment.ScopeExecution, ArtifactName: "env-v2.json"})

	if len(tc.Environments) != 1 {
		t.Fatalf("expected replacement to keep 1 env, got %d", len(tc.Environments))
	}
	if tc.Environments[0].ArtifactName != "env-v2.json" {
		t.Errorf("expected env-v2.json, got %q", tc.Environments[0].ArtifactName)
	}
}

func TestSetEnvironmentArtifact_SupportingDoesNotBecomeDefault(t *testing.T) {
	tc := &TestCase{}
	tc.SetEnvironmentArtifact(EnvironmentArtifactRef{Scope: environment.ScopeSupporting, ArtifactName: "host.json"})

	if tc.DefaultEnvironmentScope != "" {
		t.Fatalf("expected empty default without execution env, got %q", tc.DefaultEnvironmentScope)
	}
}

func TestSetTestResultArtifact_ReplacesCurrentResult(t *testing.T) {
	tc := &TestCase{}
	tc.SetTestResultArtifact(TestResultArtifactRef{ArtifactName: "result-v1.txt"})
	tc.SetTestResultArtifact(TestResultArtifactRef{ArtifactName: "result-v2.txt"})

	if tc.TestResult == nil || tc.TestResult.ArtifactName != "result-v2.txt" {
		t.Fatalf("expected result-v2.txt, got %#v", tc.TestResult)
	}
}

func TestUpsertTestScriptArtifact_AppendsAndUpdatesByName(t *testing.T) {
	tc := &TestCase{}
	tc.UpsertTestScriptArtifact(TestScriptArtifactRef{ArtifactName: "run.sh", Size: 1})
	tc.UpsertTestScriptArtifact(TestScriptArtifactRef{ArtifactName: "extra.sh", Size: 2})
	tc.UpsertTestScriptArtifact(TestScriptArtifactRef{ArtifactName: "run.sh", Size: 3})

	if len(tc.TestScripts) != 2 {
		t.Fatalf("expected 2 script refs, got %d", len(tc.TestScripts))
	}
	if tc.TestScripts[0].ArtifactName != "run.sh" || tc.TestScripts[0].Size != 3 {
		t.Fatalf("expected run.sh to be updated, got %#v", tc.TestScripts[0])
	}
}
