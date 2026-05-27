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
	output  string
	pretty  bool
	quiet   bool
	scope   string
	caseID  string
	apiBase string
	token   string
}

type detectService interface {
	DetectWithScope(context.Context, detect.Reporter, environment.EnvironmentScope) (*environment.EnvironmentSnapshot, error)
}

var (
	newDetectCommandRunner = func() command.Runner { return command.NewDefaultRunner() }
	newDetectCollectors    = defaultDetectCollectors
	newDetectService       = func(cols []environment.Collector) detectService {
		return detect.NewDetectionService(cols)
	}
)

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
  kish detect --quiet
  kish detect --case-id TC-20260505143022-a8f3 --api http://127.0.0.1:30151 --token kish_xxx --scope supporting`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDetect(cmd.Context(), flags)
		},
	}

	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Path to write the environment snapshot JSON (default: auto-generated filename)")
	cmd.Flags().BoolVar(&flags.pretty, "pretty", false, "Write indented (human-readable) JSON")
	cmd.Flags().BoolVar(&flags.quiet, "quiet", false, "Suppress progress output (errors still go to stderr)")
	cmd.Flags().StringVar(&flags.scope, "scope", string(environment.ScopeExecution), "Environment scope for the snapshot: execution or supporting")
	cmd.Flags().StringVar(&flags.caseID, "case-id", "", "Existing TestCase ID; when set, the detected environment snapshot is uploaded")
	cmd.Flags().StringVar(&flags.apiBase, "api", "", "Base URL of the kish API server (overrides KISH_API_URL)")
	cmd.Flags().StringVar(&flags.token, "token", "", "Client token for API authentication (overrides KISH_API_TOKEN)")

	return cmd
}

// runDetect executes the detection workflow and writes the result to the output file.
// Only output path and serialization errors cause the command to fail; individual
// collector failures are tolerated and reflected in the snapshot's collectors field.
func runDetect(ctx context.Context, flags detectFlags) error {
	scope, err := parseDetectScope(flags.scope)
	if err != nil {
		return err
	}

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

	runner := newDetectCommandRunner()
	cols := newDetectCollectors(runner, outputDir)
	svc := newDetectService(cols)

	snapshot, err := svc.DetectWithScope(ctx, reporter, scope)
	if err != nil {
		return fmt.Errorf("detection failed: %w", err)
	}

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

	// Detect+upload can run without leaving a local snapshot file when the
	// caller only provides --case-id. Supplying --output opts back into writing
	// the same bytes that will be uploaded.
	shouldWriteOutput := flags.output != "" || flags.caseID == ""
	if shouldWriteOutput {
		if err := os.WriteFile(outputPath, jsonBytes, 0644); err != nil {
			return fmt.Errorf("failed to write output file %q: %w", outputPath, err)
		}

		reporter.SnapshotWritten(outputPath)
	}

	if flags.caseID == "" {
		return nil
	}

	apiBase := resolveAPIBaseValue(flags.apiBase)
	if apiBase == "" {
		return fmt.Errorf("--api is required when --case-id is provided (or set KISH_API_URL)")
	}
	token := resolveTokenValue(flags.token)
	if token == "" {
		return fmt.Errorf("--token is required when --case-id is provided (or set KISH_API_TOKEN)")
	}

	artifactName := filepath.Base(outputPath)
	if err := uploadEnvironmentSnapshot(flags.caseID, artifactName, jsonBytes, apiBase, token); err != nil {
		if shouldWriteOutput {
			return fmt.Errorf("failed to upload environment snapshot for testcase %s (snapshot written to %s): %w", flags.caseID, outputPath, err)
		}
		return fmt.Errorf("failed to upload environment snapshot for testcase %s: %w", flags.caseID, err)
	}

	fmt.Printf("environment snapshot uploaded: testcase=%s scope=%s\n", flags.caseID, scope)
	return nil
}

func parseDetectScope(raw string) (environment.EnvironmentScope, error) {
	if raw == "" {
		raw = string(environment.ScopeExecution)
	}
	scope, ok := environment.NormalizeEnvironmentScope(environment.EnvironmentScope(raw))
	if !ok || !scope.IsValid() {
		return "", fmt.Errorf("invalid environment scope: %s; allowed values: execution, supporting", raw)
	}
	return scope, nil
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
