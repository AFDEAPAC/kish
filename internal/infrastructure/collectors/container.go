package collectors

import (
	"context"
	"os"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

const (
	dockerEnvPath = "/.dockerenv"
	cgroupPath    = "/proc/1/cgroup"
)

// ContainerCollector detects whether the current process is running inside a
// container by checking for the presence of /.dockerenv.
//
// It also captures /proc/1/cgroup in the raw map for debugging.
type ContainerCollector struct {
	runner        command.Runner
	dockerEnvPath string
	cgroupPath    string
}

// NewContainerCollector constructs a ContainerCollector.
func NewContainerCollector(runner command.Runner) *ContainerCollector {
	return &ContainerCollector{
		runner:        runner,
		dockerEnvPath: dockerEnvPath,
		cgroupPath:    cgroupPath,
	}
}

// Name returns the collector identifier.
func (c *ContainerCollector) Name() string { return "container" }

// SetDockerEnvPath overrides the path checked for container detection.
// Intended for use in tests only.
func (c *ContainerCollector) SetDockerEnvPath(path string) {
	c.dockerEnvPath = path
}

// SetCgroupPath overrides the path read for /proc/1/cgroup capture.
// Intended for use in tests only.
func (c *ContainerCollector) SetCgroupPath(path string) {
	c.cgroupPath = path
}

// Collect checks for container indicators and populates container.* data keys.
func (c *ContainerCollector) Collect(ctx context.Context) environment.CollectorResult {
	startedAt := time.Now()
	data := make(map[string]string)
	raw := make(map[string]string)

	// The presence of /.dockerenv is the primary container indicator for v1.
	_, err := os.Stat(c.dockerEnvPath)
	if err == nil {
		data["container.detected"] = "true"
		data["container.evidence"] = c.dockerEnvPath
	} else {
		data["container.detected"] = "false"
	}

	// Capture /proc/1/cgroup for supplementary evidence and future re-parsing.
	cgroupContent, err := os.ReadFile(c.cgroupPath)
	if err == nil && len(cgroupContent) > 0 {
		raw[c.cgroupPath] = string(cgroupContent)
	}

	return environment.CollectorResult{
		Name:       c.Name(),
		Status:     environment.StatusSuccess,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Data:       data,
		Raw:        raw,
	}
}
