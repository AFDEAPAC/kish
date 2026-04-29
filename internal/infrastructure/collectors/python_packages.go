package collectors

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

// PythonPackageCollector collects Python version information and the pip
// package list. It tries python3 first, then python as a fallback.
//
// It also detects editable/local package installations by running
// `pip list --format=json --editable` and merging the location field.
//
// Raw pip list output is written to a separate file in outputDir; the filename
// is stored in PackageSet.RawFile for traceability.
type PythonPackageCollector struct {
	runner    command.Runner
	outputDir string
}

// NewPythonPackageCollector constructs a PythonPackageCollector.
// outputDir is the directory where raw output files are written.
// Pass "." or the directory of the snapshot JSON file.
func NewPythonPackageCollector(runner command.Runner, outputDir string) *PythonPackageCollector {
	return &PythonPackageCollector{runner: runner, outputDir: outputDir}
}

// Name returns the collector identifier.
func (c *PythonPackageCollector) Name() string { return "python_packages" }

// Collect finds the Python executable, collects version/prefix info, and
// runs pip list to get the installed package list including editable installs.
func (c *PythonPackageCollector) Collect(ctx context.Context) environment.CollectorResult {
	startedAt := time.Now()

	// Determine which Python binary to use.
	pythonBin := c.resolvePythonBin()
	if pythonBin == "" {
		return environment.CollectorResult{
			Name:       c.Name(),
			Status:     environment.StatusSkipped,
			StartedAt:  startedAt,
			FinishedAt: time.Now(),
			Warnings:   []string{"neither python3 nor python found on PATH"},
		}
	}

	data := make(map[string]string)
	data["python.executable"] = pythonBin
	var warnings []string
	anyPartial := false

	// Collect version string.
	versionResult, err := c.runner.Run(ctx, pythonBin, "--version")
	if err == nil {
		// python --version prints to stdout or stderr depending on version.
		versionLine := strings.TrimSpace(versionResult.Stdout)
		if versionLine == "" {
			versionLine = strings.TrimSpace(versionResult.Stderr)
		}
		// Strip "Python " prefix: "Python 3.12.3" → "3.12.3"
		data["python.version"] = strings.TrimPrefix(versionLine, "Python ")
	} else {
		warnings = append(warnings, pythonBin+" --version failed: "+err.Error())
		anyPartial = true
	}

	// Collect sys.executable and sys.prefix.
	infoResult, err := c.runner.Run(ctx, pythonBin, "-c", "import sys; print(sys.executable); print(sys.prefix)")
	if err == nil {
		lines := strings.Split(strings.TrimSpace(infoResult.Stdout), "\n")
		if len(lines) >= 1 {
			data["python.executable"] = strings.TrimSpace(lines[0])
		}
		if len(lines) >= 2 {
			data["python.prefix"] = strings.TrimSpace(lines[1])
		}
	}

	// Capture virtualenv if present.
	if venv := os.Getenv("VIRTUAL_ENV"); venv != "" {
		data["python.virtualenv"] = venv
	}

	// Collect full pip package list.
	pipResult, err := c.runner.Run(ctx, pythonBin, "-m", "pip", "list", "--format=json")
	if err != nil {
		warnings = append(warnings, "pip list failed: "+err.Error())
		anyPartial = true

		status := environment.StatusPartial
		// Downgrade to failed only if we have no version info at all.
		if data["python.version"] == "" {
			status = environment.StatusFailed
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

	packages, parseErr := parsePipListJSON(pipResult.Stdout)
	if parseErr != nil {
		warnings = append(warnings, "could not parse pip list output: "+parseErr.Error())
		anyPartial = true
	}

	// Write raw pip list output to a separate file for traceability.
	rawFile, writeErr := WriteRawFile(c.outputDir, "piplist", startedAt, []byte(pipResult.Stdout))
	if writeErr != nil {
		warnings = append(warnings, "failed to write raw pip list output: "+writeErr.Error())
		anyPartial = true
	}

	// Collect editable package locations and merge into the package list.
	editableWarnings := c.mergeEditableLocations(ctx, pythonBin, packages)
	if len(editableWarnings) > 0 {
		warnings = append(warnings, editableWarnings...)
		anyPartial = true
	}

	pipSource := pythonBin + " -m pip list --format=json"

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
		PackageSets: []environment.PackageSet{
			{
				Type:     "python",
				Manager:  "pip",
				Source:   pipSource,
				RawFile:  rawFile,
				Packages: packages,
			},
		},
		Warnings: warnings,
	}
}

// mergeEditableLocations runs `pip list --format=json --editable`, extracts location
// information, and merges it into the provided package slice in-place.
// Returns warnings if the editable command fails; this does not fail the collector.
func (c *PythonPackageCollector) mergeEditableLocations(ctx context.Context, pythonBin string, packages []environment.Package) []string {
	result, err := c.runner.Run(ctx, pythonBin, "-m", "pip", "list", "--format=json", "--editable")
	if err != nil {
		return []string{"pip list --editable failed: " + err.Error()}
	}

	locationByName, parseErr := parsePipEditableJSON(result.Stdout)
	if parseErr != nil {
		return []string{"could not parse pip list --editable output: " + parseErr.Error()}
	}

	for i := range packages {
		// pip package names are case-insensitive and may use hyphens or underscores.
		key := normalizePipName(packages[i].Name)
		if loc, ok := locationByName[key]; ok && loc != "" {
			packages[i].Location = loc
		}
	}

	return nil
}

// resolvePythonBin returns the first Python binary found on PATH.
func (c *PythonPackageCollector) resolvePythonBin() string {
	for _, candidate := range []string{"python3", "python"} {
		if _, err := c.runner.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// pipPackageEntry matches the JSON object format used by `pip list --format=json`.
type pipPackageEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// parsePipListJSON parses the JSON array produced by `pip list --format=json`.
func parsePipListJSON(output string) ([]environment.Package, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	var entries []pipPackageEntry
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		return nil, err
	}

	packages := make([]environment.Package, 0, len(entries))
	for _, e := range entries {
		packages = append(packages, environment.Package{
			Name:    e.Name,
			Version: e.Version,
		})
	}
	return packages, nil
}

// pipEditableEntry captures the fields pip may return for editable installs.
// Different pip versions use different field names; both are handled.
type pipEditableEntry struct {
	Name                    string `json:"name"`
	EditableProjectLocation string `json:"editable_project_location"`
	Location                string `json:"location"`
}

// parsePipEditableJSON parses `pip list --format=json --editable` output and
// returns a map from normalized package name to resolved install location.
func parsePipEditableJSON(output string) (map[string]string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	var entries []pipEditableEntry
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		return nil, err
	}

	locations := make(map[string]string, len(entries))
	for _, e := range entries {
		// Prefer editable_project_location; fall back to location.
		loc := e.EditableProjectLocation
		if loc == "" {
			loc = e.Location
		}
		if loc != "" {
			locations[normalizePipName(e.Name)] = loc
		}
	}
	return locations, nil
}

// normalizePipName converts a pip package name to a canonical lookup key.
// pip treats hyphens and underscores as equivalent and names are case-insensitive.
func normalizePipName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "-", "_"))
}
