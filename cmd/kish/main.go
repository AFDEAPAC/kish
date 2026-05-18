// Command kish is the entry point for the kish CLI tool.
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
