package collectors_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AFDEAPAC/kish/internal/infrastructure/collectors"
	"github.com/AFDEAPAC/kish/internal/testutil"
)

func TestROCmCollector_VersionFileExists_VersionPopulated(t *testing.T) {
	dir := t.TempDir()
	versionFile := filepath.Join(dir, "version")
	if err := os.WriteFile(versionFile, []byte("7.1.0\n"), 0644); err != nil {
		t.Fatalf("failed to write version file: %v", err)
	}

	runner := testutil.NewFakeRunner()
	c := collectors.NewROCmCollector(runner)
	c.SetVersionFilePath(versionFile)

	result := c.Collect(context.Background())

	assertData(t, result, "rocm.version", "7.1.0")
	assertData(t, result, "rocm.version_source", versionFile)
	assertData(t, result, "rocm.info_version_file.exists", "true")
}

func TestROCmCollector_VersionFileMissing_VersionUnknown(t *testing.T) {
	runner := testutil.NewFakeRunner()
	c := collectors.NewROCmCollector(runner)
	c.SetVersionFilePath("/nonexistent/.info/version")

	result := c.Collect(context.Background())

	assertData(t, result, "rocm.version", "unknown")
	assertData(t, result, "rocm.info_version_file.exists", "false")
}

func TestROCmCollector_ROCmSMINotRunnable_ReturnsPartial(t *testing.T) {
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["rocm-smi"] = nil // available
	runner.RunResponses["rocm-smi"] = testutil.FakeRunResponse{
		Result: testutil.ExitCodeResult(1),
	}

	c := collectors.NewROCmCollector(runner)
	c.SetVersionFilePath("/nonexistent/.info/version")

	result := c.Collect(context.Background())

	assertData(t, result, "rocm_smi.available", "true")
	assertData(t, result, "rocm_smi.runnable", "false")
}
