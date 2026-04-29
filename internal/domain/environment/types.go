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

	// ScopeContext indicates an additional environment that provides context,
	// such as the host environment of a container.
	ScopeContext EnvironmentScope = "context"
)

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
