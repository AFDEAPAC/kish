package local_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AFDEAPAC/kish/internal/infrastructure/storage"
	"github.com/AFDEAPAC/kish/internal/infrastructure/storage/local"
)

func newTestStore(t *testing.T) *local.Store {
	t.Helper()
	s, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

func TestPutAndGetObject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	content := []byte("hello artifact")
	if err := s.PutObject(ctx, "testcases/tc1/artifacts/result.txt", bytes.NewReader(content), int64(len(content)), "text/plain"); err != nil {
		t.Fatalf("put: %v", err)
	}

	rc, info, err := s.GetObject(ctx, "testcases/tc1/artifacts/result.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()

	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
	if info.Size != int64(len(content)) {
		t.Errorf("size mismatch: got %d, want %d", info.Size, len(content))
	}
}

func TestPutObject_Overwrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	key := "testcases/tc1/artifacts/result.txt"

	_ = s.PutObject(ctx, key, strings.NewReader("original"), -1, "text/plain")
	_ = s.PutObject(ctx, key, strings.NewReader("updated"), -1, "text/plain")

	rc, _, err := s.GetObject(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "updated" {
		t.Errorf("expected overwritten content, got %q", got)
	}
}

func TestGetObject_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, _, err := s.GetObject(context.Background(), "nonexistent/key")
	if !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("expected ErrObjectNotFound, got %v", err)
	}
}

func TestDeleteObject_Success(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	key := "testcases/tc1/artifacts/del.txt"

	_ = s.PutObject(ctx, key, strings.NewReader("bye"), -1, "text/plain")
	if err := s.DeleteObject(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, _, err := s.GetObject(ctx, key)
	if !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("expected ErrObjectNotFound after delete, got %v", err)
	}
}

func TestDeleteObject_Missing_IsNil(t *testing.T) {
	s := newTestStore(t)
	// Deleting a non-existent key must return nil (idempotent).
	if err := s.DeleteObject(context.Background(), "testcases/tc1/artifacts/ghost.txt"); err != nil {
		t.Errorf("expected nil for missing key delete, got %v", err)
	}
}

func TestPathTraversal_Rejected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	dangerous := []string{
		"../../etc/passwd",
		"../secret",
		"testcases/../../etc/shadow",
	}
	for _, key := range dangerous {
		err := s.PutObject(ctx, key, strings.NewReader("x"), -1, "text/plain")
		if err == nil {
			t.Errorf("expected error for traversal key %q, got nil", key)
		}
	}
}

func TestRoot_IsAbsolute(t *testing.T) {
	s := newTestStore(t)
	if !filepath.IsAbs(s.Root()) {
		t.Errorf("root must be absolute, got %q", s.Root())
	}
}
