package environment

// PackageSet holds a list of packages collected from a single package manager.
//
// Raw command output is written to a separate file alongside the snapshot JSON
// rather than embedded in the snapshot, because package lists can be very large.
type PackageSet struct {
	// Type classifies the package set (e.g. "system", "python").
	Type string `json:"type"`

	// Manager identifies the package manager (e.g. "dpkg", "rpm", "pip").
	Manager string `json:"manager"`

	// Source records the command or source used to obtain the package list.
	Source string `json:"source,omitempty"`

	// RawFile is the filename of the raw command output written alongside the
	// snapshot JSON (e.g. "snapshot_dpkglist_20260429T120000Z.txt").
	// Empty if the raw file was not written.
	RawFile string `json:"raw_file,omitempty"`

	// Packages is the parsed list of packages.
	Packages []Package `json:"packages"`
}

// Package represents a single software package entry.
//
// Name is required when parsed. All other fields are optional and depend
// on the package manager.
type Package struct {
	Name string `json:"name"`

	Version string `json:"version,omitempty"`

	// Architecture is the target architecture (used mainly by dpkg/rpm).
	Architecture string `json:"architecture,omitempty"`

	// Release is the package release tag (used mainly by RPM-based managers).
	Release string `json:"release,omitempty"`

	// Location is the local filesystem path for editable or local installs.
	// Applies to Python packages only; populated from pip's editable package list.
	Location string `json:"location,omitempty"`
}
