// Package cli defines the kish CLI command tree using cobra.
//
// The CLI layer is intentionally thin: each command only handles flag parsing,
// calls the appropriate application service, and maps errors to user-facing messages.
// Business logic and data collection must not be placed here.
package cli

import (
	"github.com/spf13/cobra"
)

// Execute builds a fresh command tree and runs it with the supplied build
// version. A new tree keeps tests and repeated in-process invocations from
// sharing Cobra flag state.
func Execute(version string) error {
	return newRootCmd(version).Execute()
}

// newRootCmd assembles the CLI command tree for a single invocation.
// Tests call this directly so each case owns isolated Cobra flags and output.
func newRootCmd(version string) *cobra.Command {
	rootCmd := versionedCommand(&cobra.Command{
		Use:   "kish",
		Short: "kish — Kish platform CLI tool",
		Long: `kish is the command-line tool for the Kish platform.

It can detect and snapshot the current execution environment, and in future
versions will support uploading results and interacting with the backend API.`,
	}, version)

	rootCmd.AddCommand(versionedCommand(newDetectCmd(), version))
	rootCmd.AddCommand(versionedCommand(newAPICmd(), version))
	rootCmd.AddCommand(versionedCommand(newUploadCmd(), version))
	rootCmd.AddCommand(versionedCommand(newStorageCmd(), version))

	return rootCmd
}

// versionedCommand installs a shared version string and output template on a
// command. Cobra handles --version before validating required command flags.
func versionedCommand(cmd *cobra.Command, version string) *cobra.Command {
	cmd.Version = version
	cmd.SetVersionTemplate("{{.Version}}\n")
	return cmd
}
