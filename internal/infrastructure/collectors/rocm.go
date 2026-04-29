package collectors

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

const (
	rocmVersionFilePath = "/opt/rocm/.info/version"
)

// ROCmCollector collects ROCm version and runtime command availability.
//
// ROCm version is always read from /opt/rocm/.info/version.
// rocm-smi and rocminfo are used only to check runtime accessibility,
// not to determine the ROCm version.
type ROCmCollector struct {
	runner          command.Runner
	versionFilePath string
}

// NewROCmCollector constructs a ROCmCollector.
func NewROCmCollector(runner command.Runner) *ROCmCollector {
	return &ROCmCollector{runner: runner, versionFilePath: rocmVersionFilePath}
}

// SetVersionFilePath overrides the path used to read the ROCm version file.
// Intended for use in tests only.
func (c *ROCmCollector) SetVersionFilePath(path string) {
	c.versionFilePath = path
}

// Name returns the collector identifier.
func (c *ROCmCollector) Name() string { return "rocm" }

// Collect checks for ROCm installation and runtime tool availability.
func (c *ROCmCollector) Collect(ctx context.Context) environment.CollectorResult {
	startedAt := time.Now()
	data := make(map[string]string)
	var warnings []string
	anyPartial := false

	// Read ROCm version from the canonical version file.
	versionContent, err := os.ReadFile(c.versionFilePath)
	if err == nil {
		data["rocm.version"] = strings.TrimSpace(string(versionContent))
		data["rocm.version_source"] = c.versionFilePath
		data["rocm.info_version_file.exists"] = "true"
	} else {
		data["rocm.version"] = "unknown"
		data["rocm.info_version_file.exists"] = "false"
	}

	// Check rocm-smi availability and runnability.
	if _, err := c.runner.LookPath("rocm-smi"); err == nil {
		data["rocm_smi.available"] = "true"
		result, runErr := c.runner.Run(ctx, "rocm-smi")
		if runErr != nil || result.ExitCode != 0 {
			data["rocm_smi.runnable"] = "false"
			errMsg := buildErrMsg(runErr, result.Stderr)
			data["rocm_smi.error"] = errMsg
			warnings = append(warnings, "rocm-smi is available but not runnable: "+errMsg)
			anyPartial = true
		} else {
			data["rocm_smi.runnable"] = "true"
		}
	} else {
		data["rocm_smi.available"] = "false"
	}

	// Check rocminfo availability and runnability.
	if _, err := c.runner.LookPath("rocminfo"); err == nil {
		data["rocminfo.available"] = "true"
		result, runErr := c.runner.Run(ctx, "rocminfo")
		if runErr != nil || result.ExitCode != 0 {
			data["rocminfo.runnable"] = "false"
			errMsg := buildErrMsg(runErr, result.Stderr)
			data["rocminfo.error"] = errMsg
			warnings = append(warnings, "rocminfo is available but not runnable: "+errMsg)
			anyPartial = true
		} else {
			data["rocminfo.runnable"] = "true"
		}
	} else {
		data["rocminfo.available"] = "false"
	}

	status := environment.StatusSuccess
	if anyPartial {
		status = environment.StatusPartial
	}

	return environment.CollectorResult{
		Name:       c.Name(),
		Status:     status,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Data:       data,
		Warnings:   warnings,
	}
}

// buildErrMsg combines a Go error and a stderr string into a single message.
func buildErrMsg(err error, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if err != nil && stderr != "" {
		return err.Error() + "; stderr: " + stderr
	}
	if err != nil {
		return err.Error()
	}
	return stderr
}
