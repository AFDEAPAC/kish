// Command kish is the entry point for every kish subcommand: the API
// server, the environment-detect collector, and the artifact-upload CLI.
//
// Lifecycle responsibilities are split between this entry point and the
// individual subcommand runners under internal/cli:
//   - This file only dispatches to Cobra and translates any returned error
//     into a non-zero exit code. It does not own signal handling or
//     resource lifetime.
//   - The "api" subcommand (internal/cli/api.go runAPI) installs SIGINT /
//     SIGTERM handling, drains the HTTP server with a 10s deadline, then
//     disconnects MongoDB with a 5s deadline. Callers running kish under a
//     supervisor (systemd, Kubernetes) should rely on those handlers
//     rather than sending SIGKILL during normal shutdown.
//   - Other subcommands (upload, detect) are short-lived and exit without
//     holding long-lived resources.
package main

import (
	"fmt"
	"os"

	"github.com/AFDEAPAC/kish/internal/cli"
)

// Version is the CLI build version reported by Cobra's --version flag.
// Release builds override the development default with -ldflags "-X main.Version=...".
var Version = "development"

func main() {
	if err := cli.Execute(Version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
