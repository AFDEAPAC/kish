// Package cli defines the kish CLI command tree using cobra.
//
// The CLI layer is intentionally thin: each command only handles flag parsing,
// calls the appropriate application service, and maps errors to user-facing messages.
// Business logic and data collection must not be placed here.
package cli

import (
	"github.com/spf13/cobra"
)

// rootCmd is the top-level kish command.
var rootCmd = &cobra.Command{
	Use:   "kish",
	Short: "kish — Kish platform CLI tool",
	Long: `kish is the command-line tool for the Kish platform.

It can detect and snapshot the current execution environment, and in future
versions will support uploading results and interacting with the backend API.`,
}

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(newDetectCmd())
	rootCmd.AddCommand(newAPICmd())
	rootCmd.AddCommand(newUploadCmd())
}
