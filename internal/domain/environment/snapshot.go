package environment

import "time"

// SchemaVersionV1 is the schema_version string for the v1 environment snapshot format.
const SchemaVersionV1 = "environment-snapshot/v1"

// EnvironmentSnapshot is an immutable JSON snapshot describing the environment
// where a test was executed or a related environment context.
//
// It is produced by the kish detect command and can be uploaded to the backend
// API for association with a TestCase.
type EnvironmentSnapshot struct {
	// SchemaVersion identifies the snapshot format version.
	SchemaVersion string `json:"schema_version"`

	// KishVersion is the version of the kish CLI that produced this snapshot.
	KishVersion string `json:"kish_version"`

	// Scope describes the relationship of this snapshot to the test execution.
	Scope EnvironmentScope `json:"scope"`

	// Type describes the nature of the environment.
	Type EnvironmentType `json:"type"`

	// CollectedAt is when the snapshot was produced.
	CollectedAt time.Time `json:"collected_at"`

	// Data holds normalized, query-friendly key-value environment facts.
	// All values are strings; boolean-like values use "true"/"false"/"unknown".
	Data map[string]string `json:"data"`

	// PackageSets holds package lists from all collectors.
	PackageSets []PackageSet `json:"package_sets"`

	// Raw holds original command outputs keyed by source name.
	// This is intended for debugging and future re-parsing.
	Raw map[string]string `json:"raw"`

	// Collectors holds execution summary reports for each collector that ran.
	Collectors []CollectorReport `json:"collectors"`

	// Devices holds structured hardware device information such as per-GPU details.
	// This field is omitted from the JSON output when no device data is available.
	Devices *Devices `json:"devices,omitempty"`
}
