package collectors

import (
	"context"
	"strings"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

// rpmQueryFormat is the queryformat string for `rpm -qa`.
// Tab-delimited fields: NAME, VERSION, RELEASE, ARCH.
const rpmQueryFormat = `%{NAME}\t%{VERSION}\t%{RELEASE}\t%{ARCH}\n`

// SystemPackageCollector collects system package lists using dpkg or rpm.
// If neither is available the collector is skipped gracefully.
//
// Raw command output is written to a separate file in outputDir; the filename
// is stored in PackageSet.RawFile for traceability.
type SystemPackageCollector struct {
	runner    command.Runner
	outputDir string
}

// NewSystemPackageCollector constructs a SystemPackageCollector.
// outputDir is the directory where raw output files are written.
// Pass "." or the directory of the snapshot JSON file.
func NewSystemPackageCollector(runner command.Runner, outputDir string) *SystemPackageCollector {
	return &SystemPackageCollector{runner: runner, outputDir: outputDir}
}

func (c *SystemPackageCollector) Name() string { return "system_packages" }

// Collect detects the available package manager and collects the package list.
func (c *SystemPackageCollector) Collect(ctx context.Context) environment.CollectorResult {
	startedAt := time.Now()

	// Prefer dpkg over rpm.
	if _, err := c.runner.LookPath("dpkg"); err == nil {
		return c.collectDpkg(ctx, startedAt)
	}
	if _, err := c.runner.LookPath("rpm"); err == nil {
		return c.collectRpm(ctx, startedAt)
	}

	// Neither dpkg nor rpm available.
	return environment.CollectorResult{
		Name:       c.Name(),
		Status:     environment.StatusSkipped,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Warnings:   []string{"neither dpkg nor rpm found on PATH"},
	}
}

func (c *SystemPackageCollector) collectDpkg(ctx context.Context, startedAt time.Time) environment.CollectorResult {
	result, err := c.runner.Run(ctx, "dpkg", "-l")
	if err != nil {
		return environment.CollectorResult{
			Name:       c.Name(),
			Status:     environment.StatusFailed,
			StartedAt:  startedAt,
			FinishedAt: time.Now(),
			Error:      "dpkg -l failed: " + err.Error(),
		}
	}

	var warnings []string

	rawFile, writeErr := WriteRawFile(c.outputDir, "dpkglist", startedAt, []byte(result.Stdout))
	if writeErr != nil {
		warnings = append(warnings, "failed to write raw dpkg output: "+writeErr.Error())
	}

	packages, parseWarnings := parseDpkgOutput(result.Stdout)
	warnings = append(warnings, parseWarnings...)

	status := environment.StatusSuccess
	if len(warnings) > 0 {
		status = environment.StatusPartial
	}

	return environment.CollectorResult{
		Name:       c.Name(),
		Status:     status,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		PackageSets: []environment.PackageSet{
			{
				Type:     "system",
				Manager:  "dpkg",
				Source:   "dpkg -l",
				RawFile:  rawFile,
				Packages: packages,
			},
		},
		Warnings: warnings,
	}
}

func (c *SystemPackageCollector) collectRpm(ctx context.Context, startedAt time.Time) environment.CollectorResult {
	result, err := c.runner.Run(ctx, "rpm", "-qa", "--queryformat", rpmQueryFormat)
	if err != nil {
		return environment.CollectorResult{
			Name:       c.Name(),
			Status:     environment.StatusFailed,
			StartedAt:  startedAt,
			FinishedAt: time.Now(),
			Error:      "rpm -qa failed: " + err.Error(),
		}
	}

	var warnings []string

	rawFile, writeErr := WriteRawFile(c.outputDir, "rpmlist", startedAt, []byte(result.Stdout))
	if writeErr != nil {
		warnings = append(warnings, "failed to write raw rpm output: "+writeErr.Error())
	}

	packages, parseWarnings := parseRpmQueryOutput(result.Stdout)
	warnings = append(warnings, parseWarnings...)

	status := environment.StatusSuccess
	if len(warnings) > 0 {
		status = environment.StatusPartial
	}

	return environment.CollectorResult{
		Name:       c.Name(),
		Status:     status,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		PackageSets: []environment.PackageSet{
			{
				Type:     "system",
				Manager:  "rpm",
				Source:   "rpm -qa --queryformat",
				RawFile:  rawFile,
				Packages: packages,
			},
		},
		Warnings: warnings,
	}
}

// parseDpkgOutput parses `dpkg -l` output.
// Only lines starting with "ii" (installed) are parsed. Other lines and
// unparseable lines are skipped with a warning collected per batch.
func parseDpkgOutput(output string) ([]environment.Package, []string) {
	var packages []environment.Package
	var warnings []string
	failedLines := 0

	for _, line := range strings.Split(output, "\n") {
		// dpkg -l lines start with a 2-char status field; "ii" = installed.
		if !strings.HasPrefix(line, "ii") {
			continue
		}

		// Fields: status, name, version, architecture, description
		// Separated by variable whitespace.
		fields := strings.Fields(line)
		if len(fields) < 4 {
			failedLines++
			continue
		}

		packages = append(packages, environment.Package{
			Name:         fields[1],
			Version:      fields[2],
			Architecture: fields[3],
		})
	}

	if failedLines > 0 {
		warnings = append(warnings, "could not parse some dpkg -l lines")
	}

	return packages, warnings
}

// parseRpmQueryOutput parses `rpm -qa --queryformat '%{NAME}\t%{VERSION}\t%{RELEASE}\t%{ARCH}\n'` output.
// Each line contains tab-delimited NAME, VERSION, RELEASE, ARCH fields.
// Malformed lines are kept as raw entries and trigger a partial status.
func parseRpmQueryOutput(output string) ([]environment.Package, []string) {
	var packages []environment.Package
	var warnings []string
	failedLines := 0

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) == 4 {
			packages = append(packages, environment.Package{
				Name:         strings.TrimSpace(fields[0]),
				Version:      strings.TrimSpace(fields[1]),
				Release:      strings.TrimSpace(fields[2]),
				Architecture: strings.TrimSpace(fields[3]),
			})
		} else {
			// Keep the raw line as a best-effort entry using Name only.
			packages = append(packages, environment.Package{
				Name: line,
			})
			failedLines++
		}
	}

	if failedLines > 0 {
		warnings = append(warnings, "some rpm lines could not be fully parsed")
	}

	return packages, warnings
}
