package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommandVersionOutput(t *testing.T) {
	got := executeVersionCommand(t, []string{"--version"})
	if strings.TrimSpace(got) != "v1.2.3-test" {
		t.Fatalf("expected injected root version, got %q", got)
	}
}

func TestDirectSubcommandsVersionOutput(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "detect", args: []string{"detect", "--version"}},
		{name: "api", args: []string{"api", "--version"}},
		{name: "upload", args: []string{"upload", "--version"}},
		{name: "storage", args: []string{"storage", "--version"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := executeVersionCommand(t, tc.args)
			if strings.TrimSpace(got) != "v1.2.3-test" {
				t.Fatalf("expected injected subcommand version, got %q", got)
			}
		})
	}
}

func executeVersionCommand(t *testing.T, args []string) string {
	t.Helper()

	cmd := newRootCmd("v1.2.3-test")
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected command error: %v\nstderr:\n%s", err, errOut.String())
	}
	return out.String()
}
