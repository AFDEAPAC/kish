// Internal test package so we can access unexported helpers directly.
package cli

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultOutputPath_Format(t *testing.T) {
	ts := time.Date(2026, 4, 29, 15, 30, 22, 0, time.UTC)

	cases := []struct {
		hostname string
		want     string
	}{
		{"gpunode01", "env_gpunode01_20260429153022.json"},
		{"my-host.local", "env_my-host.local_20260429153022.json"},
		{"host with spaces", "env_host_with_spaces_20260429153022.json"},
		{"", "env_unknown-host_20260429153022.json"},
	}

	for _, tc := range cases {
		got := defaultOutputPath(tc.hostname, ts)
		if got != tc.want {
			t.Errorf("defaultOutputPath(%q, ts) = %q, want %q", tc.hostname, got, tc.want)
		}
	}
}

func TestSanitizeHostname(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"gpunode01", "gpunode01"},
		{"my-host.local", "my-host.local"},
		{"host_name", "host_name"},
		{"host with spaces", "host_with_spaces"},
		{"host@domain!com", "host_domain_com"},
		{"", ""},
	}
	for _, tc := range cases {
		got := sanitizeHostname(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeHostname(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDefaultOutputPath_ContainsExpectedParts(t *testing.T) {
	ts := time.Date(2026, 4, 29, 15, 30, 22, 0, time.UTC)
	result := defaultOutputPath("myhost", ts)
	if !strings.HasPrefix(result, "env_myhost_") {
		t.Errorf("expected prefix env_myhost_, got %q", result)
	}
	if !strings.HasSuffix(result, ".json") {
		t.Errorf("expected .json suffix, got %q", result)
	}
	if !strings.Contains(result, "20260429153022") {
		t.Errorf("expected timestamp 20260429153022 in %q", result)
	}
}
