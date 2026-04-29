package collectors_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/collectors"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
	"github.com/AFDEAPAC/kish/internal/testutil"
)

const samplePipListJSON = `[{"name": "torch", "version": "2.8.0"}, {"name": "sglang", "version": "0.4.9"}]`

// fakePythonRunner builds a FakeRunner that satisfies the common python3 detection flow.
// pipListJSON is the output returned for `python3 -m pip list --format=json`.
// editableJSON is the output returned for `python3 -m pip list --format=json --editable`.
func fakePythonRunner(pipListJSON, editableJSON string) *testutil.FakeRunner {
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["python3"] = nil
	// All python3 invocations return the pip list JSON by default.
	// Individual tests override specific responses as needed.
	runner.RunResponses["python3"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: pipListJSON},
	}
	return runner
}

func TestPythonPackageCollector_ParsesPipListJSON(t *testing.T) {
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["python3"] = nil
	runner.RunResponses["python3"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: samplePipListJSON},
	}

	c := collectors.NewPythonPackageCollector(runner, t.TempDir())
	result := c.Collect(context.Background())

	if len(result.PackageSets) == 0 {
		t.Fatal("expected at least one package set")
	}
	ps := result.PackageSets[0]
	if ps.Manager != "pip" {
		t.Errorf("expected manager=pip, got %q", ps.Manager)
	}
	if len(ps.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(ps.Packages))
	}
	if ps.Packages[0].Name != "torch" {
		t.Errorf("expected first package=torch, got %q", ps.Packages[0].Name)
	}
	if ps.Packages[0].Version != "2.8.0" {
		t.Errorf("expected torch version=2.8.0, got %q", ps.Packages[0].Version)
	}
}

func TestPythonPackageCollector_RawFileWritten(t *testing.T) {
	dir := t.TempDir()
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["python3"] = nil
	runner.RunResponses["python3"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: samplePipListJSON},
	}

	c := collectors.NewPythonPackageCollector(runner, dir)
	result := c.Collect(context.Background())

	if len(result.PackageSets) == 0 {
		t.Fatal("expected package set")
	}
	ps := result.PackageSets[0]
	if ps.RawFile == "" {
		t.Error("expected RawFile to be set")
	}
	fullPath := filepath.Join(dir, ps.RawFile)
	if _, err := os.Stat(fullPath); err != nil {
		t.Errorf("expected raw file to exist at %q: %v", fullPath, err)
	}
}

func TestPythonPackageCollector_NoPython_Skipped(t *testing.T) {
	runner := testutil.NewFakeRunner()
	// Neither python3 nor python found (default FakeRunner behaviour).

	c := collectors.NewPythonPackageCollector(runner, t.TempDir())
	result := c.Collect(context.Background())

	if result.Status != environment.StatusSkipped {
		t.Errorf("expected status=skipped, got %q", result.Status)
	}
}

func TestPythonPackageCollector_EditableWithEditableProjectLocation_SetsLocation(t *testing.T) {
	pipListOut := `[{"name": "vllm", "version": "0.8.0"}, {"name": "torch", "version": "2.8.0"}]`
	// editable_project_location field used by newer pip versions.
	editableOut := `[{"name": "vllm", "version": "0.8.0", "editable_project_location": "/workspace/vllm"}]`

	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["python3"] = nil

	// The collector calls python3 multiple times; we need separate responses.
	// FakeRunner maps by name only, so we need to detect which call is which.
	// We use a custom runner that differentiates by args.
	smartRunner := &smartFakeRunner{
		lookPathResponses: map[string]error{"python3": nil},
		runResponses: map[string]command.CommandResult{
			"version":  {Stdout: "Python 3.12.3\n"},
			"info":     {Stdout: "/usr/bin/python3\n/usr\n"},
			"piplist":  {Stdout: pipListOut},
			"editable": {Stdout: editableOut},
		},
	}

	c := collectors.NewPythonPackageCollector(smartRunner, t.TempDir())
	result := c.Collect(context.Background())

	if len(result.PackageSets) == 0 {
		t.Fatal("expected package set")
	}
	pkgs := result.PackageSets[0].Packages
	var vllm *environment.Package
	for i := range pkgs {
		if pkgs[i].Name == "vllm" {
			vllm = &pkgs[i]
		}
	}
	if vllm == nil {
		t.Fatal("vllm not found in packages")
	}
	if vllm.Location != "/workspace/vllm" {
		t.Errorf("expected location=/workspace/vllm, got %q", vllm.Location)
	}
}

func TestPythonPackageCollector_EditableWithLocationField_SetsLocation(t *testing.T) {
	pipListOut := `[{"name": "aiter", "version": "0.1.0"}]`
	// location field used by older pip versions.
	editableOut := `[{"name": "aiter", "version": "0.1.0", "location": "/workspace/aiter"}]`

	smartRunner := &smartFakeRunner{
		lookPathResponses: map[string]error{"python3": nil},
		runResponses: map[string]command.CommandResult{
			"version":  {Stdout: "Python 3.11.0\n"},
			"info":     {Stdout: "/usr/bin/python3\n/usr\n"},
			"piplist":  {Stdout: pipListOut},
			"editable": {Stdout: editableOut},
		},
	}

	c := collectors.NewPythonPackageCollector(smartRunner, t.TempDir())
	result := c.Collect(context.Background())

	if len(result.PackageSets) == 0 || len(result.PackageSets[0].Packages) == 0 {
		t.Fatal("expected packages")
	}
	pkg := result.PackageSets[0].Packages[0]
	if pkg.Location != "/workspace/aiter" {
		t.Errorf("expected location=/workspace/aiter, got %q", pkg.Location)
	}
}

func TestPythonPackageCollector_EditableCommandFails_ReturnsPartial(t *testing.T) {
	pipListOut := `[{"name": "torch", "version": "2.8.0"}]`

	smartRunner := &smartFakeRunner{
		lookPathResponses: map[string]error{"python3": nil},
		runResponses: map[string]command.CommandResult{
			"version": {Stdout: "Python 3.12.3\n"},
			"info":    {Stdout: "/usr/bin/python3\n/usr\n"},
			"piplist": {Stdout: pipListOut},
			// editable key absent → will trigger error path
		},
		editableErr: errors.New("pip: --editable not supported"),
	}

	c := collectors.NewPythonPackageCollector(smartRunner, t.TempDir())
	result := c.Collect(context.Background())

	// Editable failure → partial; main package list should still be present.
	if result.Status != environment.StatusPartial {
		t.Errorf("expected status=partial when editable fails, got %q", result.Status)
	}
	if len(result.PackageSets) == 0 || len(result.PackageSets[0].Packages) == 0 {
		t.Error("expected main package list to still be present")
	}
}

// smartFakeRunner differentiates python3 calls by inspecting the args slice.
// It is used only in tests where the FakeRunner's single-key map is insufficient.
type smartFakeRunner struct {
	lookPathResponses map[string]error
	runResponses      map[string]command.CommandResult
	editableErr       error
}

func (r *smartFakeRunner) LookPath(file string) (string, error) {
	if err, ok := r.lookPathResponses[file]; ok {
		return "/usr/bin/" + file, err
	}
	return "", errors.New(file + ": not found")
}

func (r *smartFakeRunner) Run(_ context.Context, name string, args ...string) (command.CommandResult, error) {
	key := r.classifyArgs(args)
	if key == "editable" && r.editableErr != nil {
		return command.CommandResult{}, r.editableErr
	}
	if res, ok := r.runResponses[key]; ok {
		return res, nil
	}
	return command.CommandResult{}, nil
}

func (r *smartFakeRunner) classifyArgs(args []string) string {
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	switch {
	case contains(args, "--version"):
		return "version"
	case contains(args, "-c"):
		return "info"
	case contains(args, "--editable"):
		return "editable"
	case contains(args, "list"):
		return "piplist"
	default:
		return joined
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
