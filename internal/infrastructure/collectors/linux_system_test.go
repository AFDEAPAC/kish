package collectors_test

import (
	"context"
	"os"
	"testing"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/collectors"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
	"github.com/AFDEAPAC/kish/internal/testutil"
)

const sampleOSRelease = `NAME="Ubuntu"
ID=ubuntu
VERSION="24.04 LTS (Noble Numbat)"
VERSION_ID="24.04"
PRETTY_NAME="Ubuntu 24.04 LTS"
`

func TestLinuxSystemCollector_ParsesOSRelease(t *testing.T) {
	f := writeTempFile(t, sampleOSRelease)

	runner := testutil.NewFakeRunner()
	runner.RunResponses["uname"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: "Linux\n"},
	}
	runner.LookPathResponses["hostname"] = nil

	c := collectors.NewLinuxSystemCollector(runner)
	c.SetOSReleasePath(f)

	result := c.Collect(context.Background())

	assertData(t, result, "os.name", "Ubuntu")
	assertData(t, result, "os.id", "ubuntu")
	assertData(t, result, "os.version", "24.04 LTS (Noble Numbat)")
	assertData(t, result, "os.version_id", "24.04")
}

func TestLinuxSystemCollector_MissingOSRelease_ReturnsPartial(t *testing.T) {
	runner := testutil.NewFakeRunner()
	runner.RunResponses["uname"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: "Linux\n"},
	}

	c := collectors.NewLinuxSystemCollector(runner)
	c.SetOSReleasePath("/nonexistent/path/os-release")

	result := c.Collect(context.Background())

	if result.Status != environment.StatusPartial {
		t.Errorf("expected status=%q, got %q", environment.StatusPartial, result.Status)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning for missing os-release")
	}
}

// writeTempFile writes content to a temp file and returns its path.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

// assertData asserts that result.Data[key] == expected.
func assertData(t *testing.T, result environment.CollectorResult, key, expected string) {
	t.Helper()
	if got := result.Data[key]; got != expected {
		t.Errorf("data[%q]: expected %q, got %q", key, expected, got)
	}
}
