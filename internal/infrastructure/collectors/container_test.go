package collectors_test

import (
	"context"
	"os"
	"testing"

	"github.com/AFDEAPAC/kish/internal/infrastructure/collectors"
	"github.com/AFDEAPAC/kish/internal/testutil"
)

func TestContainerCollector_DockerenvPresent_DetectedTrue(t *testing.T) {
	// Create a temporary file that stands in for /.dockerenv.
	f, err := os.CreateTemp(t.TempDir(), ".dockerenv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	f.Close()

	runner := testutil.NewFakeRunner()
	c := collectors.NewContainerCollector(runner)
	c.SetDockerEnvPath(f.Name())
	c.SetCgroupPath("/nonexistent/cgroup") // suppress cgroup read

	result := c.Collect(context.Background())

	assertData(t, result, "container.detected", "true")
	assertData(t, result, "container.evidence", f.Name())
}

func TestContainerCollector_DockerenvAbsent_DetectedFalse(t *testing.T) {
	runner := testutil.NewFakeRunner()
	c := collectors.NewContainerCollector(runner)
	c.SetDockerEnvPath("/nonexistent/.dockerenv")
	c.SetCgroupPath("/nonexistent/cgroup")

	result := c.Collect(context.Background())

	assertData(t, result, "container.detected", "false")
}
