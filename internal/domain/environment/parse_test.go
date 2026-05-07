package environment

import (
	"errors"
	"testing"
)

func TestParseSnapshot_ValidExecution(t *testing.T) {
	snap, err := ParseSnapshot([]byte(`{"schema_version":"environment-snapshot/v1","scope":"execution","type":"container","collected_at":"2026-05-06T13:24:15.654398172Z","data":{}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Scope != ScopeExecution {
		t.Errorf("expected execution scope, got %q", snap.Scope)
	}
}

func TestParseSnapshot_ValidSupporting(t *testing.T) {
	snap, err := ParseSnapshot([]byte(`{"schema_version":"environment-snapshot/v1","scope":"supporting","type":"host","collected_at":"2026-05-06T13:24:15Z","data":{}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Scope != ScopeSupporting {
		t.Errorf("expected supporting scope, got %q", snap.Scope)
	}
}

func TestParseSnapshot_NormalizesLegacyContext(t *testing.T) {
	snap, err := ParseSnapshot([]byte(`{"schema_version":"environment-snapshot/v1","scope":"context","type":"host","collected_at":"2026-05-06T13:24:15Z","data":{}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Scope != ScopeSupporting {
		t.Errorf("expected context to normalize to supporting, got %q", snap.Scope)
	}
}

func TestParseSnapshot_MissingScope(t *testing.T) {
	_, err := ParseSnapshot([]byte(`{"schema_version":"environment-snapshot/v1","type":"container","collected_at":"2026-05-06T13:24:15Z","data":{}}`))
	if !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("expected ErrInvalidSnapshot, got %v", err)
	}
}

func TestParseSnapshot_UnsupportedSchemaVersion(t *testing.T) {
	_, err := ParseSnapshot([]byte(`{"schema_version":"environment-snapshot/v2","scope":"execution","type":"container","collected_at":"2026-05-06T13:24:15Z","data":{}}`))
	if !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("expected ErrInvalidSnapshot, got %v", err)
	}
}

func TestParseSnapshot_InvalidCollectedAt(t *testing.T) {
	_, err := ParseSnapshot([]byte(`{"schema_version":"environment-snapshot/v1","scope":"execution","type":"container","collected_at":"not-time","data":{}}`))
	if !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("expected ErrInvalidSnapshot, got %v", err)
	}
}

func TestParseSnapshot_UnknownScope(t *testing.T) {
	_, err := ParseSnapshot([]byte(`{"schema_version":"environment-snapshot/v1","scope":"mystery","type":"container","collected_at":"2026-05-06T13:24:15Z","data":{}}`))
	if !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("expected ErrInvalidSnapshot, got %v", err)
	}
}
