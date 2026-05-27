// Package collectors provides infrastructure-layer implementations of the
// environment.Collector interface. Each file implements one collector that
// gathers a specific category of environment data.
//
// Cross-cutting assumptions:
//   - Collectors target Linux hosts. Behaviour on macOS or Windows is
//     undefined; the kish detect CLI is the only intended caller and it is
//     packaged as a Linux binary.
//   - External commands are invoked through a command.Runner port so the
//     CLI can inject the default runner (5-second per-command timeout via
//     command.DefaultCommandTimeout) and tests can inject a fake.
//   - Failures are reported as warnings or partial/failed CollectorStatus
//     rather than Go errors; the snapshot must still serialise successfully
//     even when individual collectors cannot gather data.
//   - Collectors must not retain state across Collect calls; the runner and
//     output directory are the only fields that survive between invocations.
package collectors

import (
	"bufio"
	"context"
	"os"
	"strings"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

const osReleasePath = "/etc/os-release"

// LinuxSystemCollector collects basic Linux system information: OS identity,
// kernel details, architecture, and hostname.
type LinuxSystemCollector struct {
	runner command.Runner
	// osReleasePath allows tests to override the path to /etc/os-release.
	osReleasePath string
}

// NewLinuxSystemCollector constructs a LinuxSystemCollector wired to the
// default /etc/os-release path. Use SetOSReleasePath in tests to point at a
// fixture.
func NewLinuxSystemCollector(runner command.Runner) *LinuxSystemCollector {
	return &LinuxSystemCollector{runner: runner, osReleasePath: osReleasePath}
}

// SetOSReleasePath overrides the path used to read /etc/os-release.
// This is intended for use in tests only.
func (c *LinuxSystemCollector) SetOSReleasePath(path string) {
	c.osReleasePath = path
}

func (c *LinuxSystemCollector) Name() string { return "linux_system" }

// Collect gathers OS, kernel, architecture, and hostname information.
// Returns partial if some sources are unavailable; success if all succeed.
func (c *LinuxSystemCollector) Collect(ctx context.Context) environment.CollectorResult {
	startedAt := time.Now()
	data := make(map[string]string)
	var warnings []string
	anyFailed := false

	osFields, err := parseOSRelease(c.osReleasePath)
	if err != nil {
		warnings = append(warnings, "could not read "+c.osReleasePath+": "+err.Error())
		anyFailed = true
	} else {
		if v, ok := osFields["NAME"]; ok {
			data["os.name"] = v
		}
		if v, ok := osFields["ID"]; ok {
			data["os.id"] = v
		}
		if v, ok := osFields["VERSION"]; ok {
			data["os.version"] = v
		}
		if v, ok := osFields["VERSION_ID"]; ok {
			data["os.version_id"] = v
		}
	}

	type unameCmd struct {
		flag string
		key  string
	}
	unameCmds := []unameCmd{
		{"-s", "kernel.name"},
		{"-r", "kernel.release"},
		{"-v", "kernel.version"},
		{"-m", "architecture"},
	}
	for _, u := range unameCmds {
		result, err := c.runner.Run(ctx, "uname", u.flag)
		if err != nil {
			warnings = append(warnings, "uname "+u.flag+" failed: "+err.Error())
			anyFailed = true
			continue
		}
		data[u.key] = strings.TrimSpace(result.Stdout)
	}

	hostname, err := os.Hostname()
	if err != nil {
		// Fallback to the hostname command.
		result, cmdErr := c.runner.Run(ctx, "hostname")
		if cmdErr != nil {
			warnings = append(warnings, "hostname unavailable: "+err.Error())
			anyFailed = true
		} else {
			data["hostname"] = strings.TrimSpace(result.Stdout)
		}
	} else {
		data["hostname"] = hostname
	}

	status := environment.StatusSuccess
	if anyFailed {
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

// parseOSRelease reads a KEY="VALUE" formatted file and returns a map of entries.
// Quotes around values are stripped. Lines starting with # are ignored.
func parseOSRelease(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fields := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		fields[key] = value
	}
	return fields, scanner.Err()
}
