package artifact_test

import (
	"testing"

	"github.com/AFDEAPAC/kish/internal/domain/artifact"
)

func TestValidateArtifactName_ValidNames(t *testing.T) {
	valid := []string{
		"result.txt",
		"env.json",
		"run.sh",
		"snapshot_dpkglist_20260506T120000Z.txt",
		"benchmark.log",
		"a",
		"A-B_C.d",
		"file123",
	}
	for _, name := range valid {
		if err := artifact.ValidateArtifactName(name); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", name, err)
		}
	}
}

func TestValidateArtifactName_InvalidNames(t *testing.T) {
	cases := []struct {
		name   string
		reason string
	}{
		{"", "empty string"},
		{".", "dot"},
		{"..", "dotdot"},
		{"../etc/passwd", "path traversal with dotdot"},
		{"logs/result.txt", "contains slash"},
		{`logs\result.txt`, "contains backslash"},
		{"file\x00name", "contains null byte"},
		{"file name", "contains space"},
		{"file@name", "contains @"},
		{"résumé.txt", "contains non-ASCII"},
	}
	for _, tc := range cases {
		if err := artifact.ValidateArtifactName(tc.name); err == nil {
			t.Errorf("expected %q (%s) to be invalid, but got no error", tc.name, tc.reason)
		}
	}
}

func TestValidateArtifactName_TooLong(t *testing.T) {
	name := make([]byte, 256)
	for i := range name {
		name[i] = 'a'
	}
	if err := artifact.ValidateArtifactName(string(name)); err == nil {
		t.Error("expected error for name longer than 255 chars")
	}
}

func TestValidateArtifactName_MaxLength(t *testing.T) {
	name := make([]byte, 255)
	for i := range name {
		name[i] = 'a'
	}
	if err := artifact.ValidateArtifactName(string(name)); err != nil {
		t.Errorf("expected 255-char name to be valid, got: %v", err)
	}
}

func TestStorageKeyFor(t *testing.T) {
	key := artifact.StorageKeyFor("case_abc123", "result.txt")
	expected := "testcases/case_abc123/artifacts/result.txt"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

func TestIsValidArtifactType(t *testing.T) {
	valid := []artifact.ArtifactType{
		artifact.ArtifactTypeEnvironment,
		artifact.ArtifactTypeResult,
		artifact.ArtifactTypeScript,
		artifact.ArtifactTypeRaw,
		artifact.ArtifactTypeLog,
		artifact.ArtifactTypeOther,
	}
	for _, t2 := range valid {
		if !artifact.IsValidArtifactType(t2) {
			t.Errorf("expected %q to be valid artifact type", t2)
		}
	}
	if artifact.IsValidArtifactType("unknown-type") {
		t.Error("expected 'unknown-type' to be invalid")
	}
}
