package environment

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidSnapshot marks validation errors in uploaded environment snapshots.
// Handlers use this sentinel to return a client-facing 400 instead of a generic
// storage failure.
var ErrInvalidSnapshot = errors.New("invalid environment snapshot")

// ParseSnapshot decodes and validates an uploaded EnvironmentSnapshot JSON
// payload. It normalizes legacy scope spellings so the rest of the domain only
// has to deal with canonical values.
func ParseSnapshot(data []byte) (*EnvironmentSnapshot, error) {
	var snap EnvironmentSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %v", ErrInvalidSnapshot, err)
	}
	if err := ValidateSnapshot(&snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// ValidateSnapshot enforces the minimum EnvironmentSnapshot contract required
// by the backend to link an environment artifact to a TestCase.
func ValidateSnapshot(snap *EnvironmentSnapshot) error {
	if snap == nil {
		return fmt.Errorf("%w: snapshot is required", ErrInvalidSnapshot)
	}
	if snap.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("%w: unsupported schema_version %q", ErrInvalidSnapshot, snap.SchemaVersion)
	}
	scope, ok := NormalizeEnvironmentScope(snap.Scope)
	if !ok {
		return fmt.Errorf("%w: unsupported scope %q", ErrInvalidSnapshot, snap.Scope)
	}
	snap.Scope = scope
	if snap.Type == "" {
		return fmt.Errorf("%w: type is required", ErrInvalidSnapshot)
	}
	if !snap.Type.IsValid() {
		return fmt.Errorf("%w: unsupported type %q", ErrInvalidSnapshot, snap.Type)
	}
	if snap.CollectedAt.IsZero() {
		return fmt.Errorf("%w: collected_at is required", ErrInvalidSnapshot)
	}
	return nil
}
