// Package testutil provides shared test helpers.
// It must only be imported from _test.go files or test binaries.
package testutil

import (
	"context"
	"fmt"

	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

// FakeCall records one invocation of FakeRunner.Run or FakeRunner.LookPath.
type FakeCall struct {
	Name string
	Args []string
}

// FakeRunResponse defines the canned response for a specific command name.
type FakeRunResponse struct {
	Result command.CommandResult
	Err    error
}

// FakeRunner is a test double for command.Runner that returns pre-configured
// responses without executing real OS commands.
type FakeRunner struct {
	// LookPathResponses maps binary names to errors (nil = found, non-nil = not found).
	LookPathResponses map[string]error

	// RunResponses maps command names to canned responses.
	RunResponses map[string]FakeRunResponse

	// Calls records all LookPath and Run invocations for assertion in tests.
	Calls []FakeCall
}

// NewFakeRunner constructs an empty FakeRunner. Populate LookPathResponses and
// RunResponses before use.
func NewFakeRunner() *FakeRunner {
	return &FakeRunner{
		LookPathResponses: make(map[string]error),
		RunResponses:      make(map[string]FakeRunResponse),
	}
}

// LookPath returns a pre-configured error for the given binary name.
// If no entry exists, it returns an "not found" error.
func (r *FakeRunner) LookPath(file string) (string, error) {
	r.Calls = append(r.Calls, FakeCall{Name: "LookPath:" + file})
	if err, ok := r.LookPathResponses[file]; ok {
		if err == nil {
			return "/usr/bin/" + file, nil
		}
		return "", err
	}
	return "", fmt.Errorf("%s: not found", file)
}

// Run returns a pre-configured response for the given command name.
// If no entry exists, it returns an empty successful result.
func (r *FakeRunner) Run(ctx context.Context, name string, args ...string) (command.CommandResult, error) {
	r.Calls = append(r.Calls, FakeCall{Name: name, Args: args})
	if resp, ok := r.RunResponses[name]; ok {
		return resp.Result, resp.Err
	}
	return command.CommandResult{}, nil
}

// ExitCodeResult is a helper that returns a CommandResult with the given exit code.
func ExitCodeResult(code int) command.CommandResult {
	return command.CommandResult{ExitCode: code}
}
