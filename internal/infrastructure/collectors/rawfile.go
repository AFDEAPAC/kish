package collectors

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WriteRawFile writes content to outputDir/snapshot_<name>_<timestamp>.txt and
// returns only the filename (e.g. "snapshot_dpkglist_20260429T120000Z.txt").
//
// The returned filename is stored in PackageSet.RawFile to allow callers to
// reference the file relative to the snapshot JSON directory.
//
// If outputDir is empty, the current working directory is used.
// An error is returned if the directory cannot be created or the file cannot be written,
// but the error is non-fatal from the collector's perspective: the caller may
// record a warning and proceed without a raw file.
func WriteRawFile(outputDir, name string, t time.Time, content []byte) (string, error) {
	if outputDir == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory %q: %w", outputDir, err)
	}

	filename := fmt.Sprintf("snapshot_%s_%s.txt", name, t.UTC().Format("20060102T150405Z"))
	fullPath := filepath.Join(outputDir, filename)

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write raw file %q: %w", fullPath, err)
	}
	return filename, nil
}
