package environment

// Devices holds structured hardware device information collected during detection.
//
// Per-device data is stored here rather than as comma-separated strings in the
// top-level data map. Summary fields (e.g. gpu.count, gpu.vendor) remain in
// data for quick querying.
type Devices struct {
	// GPUs is the list of detected GPU devices, one entry per physical device.
	GPUs []GPUDevice `json:"gpus,omitempty"`
}

// GPUDevice represents a single detected GPU.
//
// Index is the stable position within the detected output (0-based).
// ID is the PCI bus address when available (e.g. "0000:03:00.0").
type GPUDevice struct {
	// Index is the 0-based position of this GPU in the detected output.
	Index int `json:"index"`

	// Model is the GPU product name (e.g. "AMD Instinct MI300X").
	Model string `json:"model,omitempty"`

	// Vendor is the GPU manufacturer (e.g. "AMD", "NVIDIA").
	Vendor string `json:"vendor,omitempty"`

	// ID is the PCI bus address. Omitted when not available.
	ID string `json:"id,omitempty"`
}
