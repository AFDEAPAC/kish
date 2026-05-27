// Package command provides an abstraction over OS command execution.
//
// Using this abstraction makes collectors testable by injecting a fake runner
// instead of requiring real binaries in unit tests.
package command

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// DefaultCommandTimeout is applied to every command run by DefaultRunner.
// This prevents hanging commands from blocking the detection process indefinitely.
const DefaultCommandTimeout = 5 * time.Second

// CommandResult holds the captured output and exit code of a completed command.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner is the interface used by collectors to discover and execute commands.
//
// Implementing a fake Runner in tests allows collectors to be tested without
// requiring real binaries or a specific OS environment.
type Runner interface {
	// LookPath reports whether a binary is available on PATH, returning its
	// resolved path or an error if not found.
	LookPath(file string) (string, error)

	// Run executes the named command with the given arguments and returns the
	// captured output. The provided context is used for cancellation; a
	// per-command timeout is also applied by DefaultRunner.
	Run(ctx context.Context, name string, args ...string) (CommandResult, error)
}

// DefaultRunner executes real OS commands.
// Each Run call wraps the provided context with DefaultCommandTimeout so that
// slow or hanging commands do not block detection indefinitely.
type DefaultRunner struct{}

// NewDefaultRunner constructs a DefaultRunner.
func NewDefaultRunner() *DefaultRunner {
	return &DefaultRunner{}
}

// LookPath resolves the full path of a binary using the system PATH.
func (r *DefaultRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// Run applies DefaultCommandTimeout on top of ctx so real collector commands
// cannot hang detection indefinitely.
func (r *DefaultRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	return CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, err
}
