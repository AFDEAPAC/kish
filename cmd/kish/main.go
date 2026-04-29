// Command kish is the entry point for the kish CLI tool.
package main

import (
	"fmt"
	"os"

	"github.com/AFDEAPAC/kish/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
