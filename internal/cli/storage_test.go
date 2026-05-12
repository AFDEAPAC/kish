package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStorageCheckLocalBackend(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "artifacts")
	cfgPath := filepath.Join(dir, "kish.yaml")
	yamlContent := `
storage:
  type: "local"
  local:
    root: "` + root + `"
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := runStorageCheck(t.Context(), &out, storageCheckFlags{configPath: cfgPath})
	if err != nil {
		t.Fatalf("unexpected storage check error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"storage type: local",
		"[OK] initialized storage backend",
		"[OK] put diagnostic object",
		"[OK] delete diagnostic object",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestStorageCheckCommandRegistersConfigFlag(t *testing.T) {
	cmd := newStorageCheckCmd()
	if cmd.Flags().Lookup("config") == nil {
		t.Fatal("expected --config flag")
	}
}
