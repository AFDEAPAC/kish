// Package environment defines the core domain types for environment snapshots.
//
// These types represent the shared contract between the kish CLI collector
// and the backend API. They are intentionally kept free of infrastructure
// concerns such as file I/O or command execution.
package environment

// EnvironmentScope describes the relationship of an environment snapshot
// to the test it belongs to.
type EnvironmentScope string

const (
	// ScopeExecution indicates the environment where the test command actually ran.
	ScopeExecution EnvironmentScope = "execution"

	// ScopeSupporting indicates an additional environment that supports the
	// execution environment, such as the host environment of a container.
	ScopeSupporting EnvironmentScope = "supporting"

	// ScopeContext is a legacy spelling accepted on input and normalized to
	// ScopeSupporting. New snapshots should emit "supporting".
	ScopeContext EnvironmentScope = "context"
)

// NormalizeEnvironmentScope converts accepted input spellings to the canonical
// domain scope. The legacy "context" value is preserved as an input alias only.
func NormalizeEnvironmentScope(scope EnvironmentScope) (EnvironmentScope, bool) {
	switch scope {
	case ScopeExecution:
		return ScopeExecution, true
	case ScopeSupporting, ScopeContext:
		return ScopeSupporting, true
	}
	return "", false
}

// IsValid reports whether scope is a canonical EnvironmentScope value.
func (scope EnvironmentScope) IsValid() bool {
	switch scope {
	case ScopeExecution, ScopeSupporting:
		return true
	}
	return false
}

// EnvironmentType describes the nature of the environment.
type EnvironmentType string

const (
	// TypeContainer indicates a container (Docker, OCI, etc.).
	TypeContainer EnvironmentType = "container"

	// TypeHost indicates a general host environment not further classified.
	// Used when the environment is not a container and exact type is not determined.
	TypeHost EnvironmentType = "host"

	// TypeBareMetal indicates a physical server with no hypervisor.
	// Reserved for future detection logic.
	TypeBareMetal EnvironmentType = "bare_metal"

	// TypeVirtualMachine indicates a virtual machine (KVM, QEMU, VMware, etc.).
	// Reserved for future detection logic.
	TypeVirtualMachine EnvironmentType = "virtual_machine"

	// TypeUnknown indicates the environment type could not be determined.
	TypeUnknown EnvironmentType = "unknown"
)

// IsValid reports whether t is a recognized EnvironmentType.
func (t EnvironmentType) IsValid() bool {
	switch t {
	case TypeContainer, TypeHost, TypeBareMetal, TypeVirtualMachine, TypeUnknown:
		return true
	}
	return false
}

// CollectorStatus describes the outcome of a single collector run.
type CollectorStatus string

const (
	// StatusSuccess indicates the collector completed successfully.
	StatusSuccess CollectorStatus = "success"

	// StatusPartial indicates the collector collected some data but one or
	// more sub-steps failed (e.g. a command returned non-zero).
	StatusPartial CollectorStatus = "partial"

	// StatusSkipped indicates the collector was not applicable, typically
	// because a required binary is missing.
	StatusSkipped CollectorStatus = "skipped"

	// StatusFailed indicates the collector encountered a fatal internal error.
	// The detection process continues regardless.
	StatusFailed CollectorStatus = "failed"
)
