package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/spf13/cobra"

	"github.com/AFDEAPAC/kish/internal/application/detect"
	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

// detectFlags holds parsed CLI flags for the detect subcommand.
type detectFlags struct {
	output string
	pretty bool
	quiet  bool
}

// newDetectCmd constructs the `kish detect` cobra command.
func newDetectCmd() *cobra.Command {
	var flags detectFlags

	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Detect the current execution environment and write a snapshot JSON",
		Long: `detect collects information about the current Linux environment —
including OS, kernel, installed packages, ROCm version, GPU details, and
Python packages — and writes an environment-snapshot/v1 JSON file.

When --output is omitted, the file is named automatically:
  env_{hostname}_{yyyymmddHHMMSS}.json

Examples:
  kish detect
  kish detect --output env.json
  kish detect --output env.json --pretty
  kish detect --quiet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDetect(cmd.Context(), flags)
		},
	}

	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Path to write the environment snapshot JSON (default: auto-generated filename)")
	cmd.Flags().BoolVar(&flags.pretty, "pretty", false, "Write indented (human-readable) JSON")
	cmd.Flags().BoolVar(&flags.quiet, "quiet", false, "Suppress progress output (errors still go to stderr)")

	return cmd
}

// runDetect executes the detection workflow and writes the result to the output file.
// Only output path and serialization errors cause the command to fail; individual
// collector failures are tolerated and reflected in the snapshot's collectors field.
func runDetect(ctx context.Context, flags detectFlags) error {
	var reporter detect.Reporter
	if flags.quiet {
		reporter = &detect.NoopReporter{}
	} else {
		reporter = &detect.StdoutReporter{}
	}

	// Raw output files are written alongside the snapshot JSON.
	// When --output is given, use its directory; otherwise use the current directory.
	outputDir := "."
	if flags.output != "" {
		outputDir = filepath.Dir(flags.output)
	}

	runner := command.NewDefaultRunner()
	cols := detect.DefaultCollectors(runner, outputDir)
	svc := detect.NewDetectionService(cols)

	snapshot, err := svc.Detect(ctx, reporter)
	if err != nil {
		return fmt.Errorf("detection failed: %w", err)
	}

	// Resolve output path: use the provided flag or generate a default filename.
	outputPath := flags.output
	if outputPath == "" {
		outputPath = defaultOutputPath(snapshotHostname(snapshot), snapshot.CollectedAt)
	}

	var jsonBytes []byte
	if flags.pretty {
		jsonBytes, err = json.MarshalIndent(snapshot, "", "  ")
	} else {
		jsonBytes, err = json.Marshal(snapshot)
	}
	if err != nil {
		return fmt.Errorf("failed to serialize snapshot: %w", err)
	}

	if err := os.WriteFile(outputPath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("failed to write output file %q: %w", outputPath, err)
	}

	reporter.SnapshotWritten(outputPath)
	return nil
}

// snapshotHostname extracts the hostname for the default output filename.
// Priority: snapshot.Data["hostname"] → os.Hostname() → "unknown-host".
func snapshotHostname(snap *environment.EnvironmentSnapshot) string {
	if h, ok := snap.Data["hostname"]; ok && h != "" {
		return h
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown-host"
}

// defaultOutputPath generates the default output filename.
// Format: env_{sanitized_hostname}_{yyyymmddHHMMSS}.json
func defaultOutputPath(hostname string, t time.Time) string {
	if hostname == "" {
		hostname = "unknown-host"
	}
	return fmt.Sprintf("env_%s_%s.json", sanitizeHostname(hostname), t.Format("20060102150405"))
}

// unsafeHostnameChars matches characters not safe for use in filenames.
var unsafeHostnameChars = regexp.MustCompile(`[^a-zA-Z0-9._\-]`)

// sanitizeHostname replaces characters not in [a-zA-Z0-9._-] with underscores.
func sanitizeHostname(h string) string {
	return unsafeHostnameChars.ReplaceAllString(h, "_")
}
