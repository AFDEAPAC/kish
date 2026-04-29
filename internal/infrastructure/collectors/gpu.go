package collectors

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

// GPUCollector collects GPU information.
//
// Detection priority:
//  1. rocm-smi (if available and runnable)
//  2. lspci fallback
//
// Absence of a GPU does not constitute a failure; the collector returns
// success with gpu.count = "0". Per-device details are returned in
// CollectorResult.Devices rather than as comma-separated strings in data.
type GPUCollector struct {
	runner command.Runner
}

// NewGPUCollector constructs a GPUCollector.
func NewGPUCollector(runner command.Runner) *GPUCollector {
	return &GPUCollector{runner: runner}
}

// Name returns the collector identifier.
func (c *GPUCollector) Name() string { return "gpu" }

// Collect detects GPUs using rocm-smi or lspci.
func (c *GPUCollector) Collect(ctx context.Context) environment.CollectorResult {
	startedAt := time.Now()
	data := make(map[string]string)
	var warnings []string

	// Try rocm-smi first.
	if _, err := c.runner.LookPath("rocm-smi"); err == nil {
		result, runErr := c.runner.Run(ctx, "rocm-smi", "--showproductname")
		if runErr == nil && result.ExitCode == 0 {
			devices := parseROCmSMIDevices(result.Stdout)
			populateGPUSummary(devices, data)
			return environment.CollectorResult{
				Name:       c.Name(),
				Status:     environment.StatusSuccess,
				StartedAt:  startedAt,
				FinishedAt: time.Now(),
				Data:       data,
				Devices:    toDevices(devices),
			}
		}
		warnings = append(warnings, "rocm-smi --showproductname failed, falling back to lspci")
	}

	// Fallback: lspci.
	if _, err := c.runner.LookPath("lspci"); err == nil {
		result, runErr := c.runner.Run(ctx, "lspci")
		if runErr == nil {
			devices := parseLspciDevices(result.Stdout)
			populateGPUSummary(devices, data)
			if len(devices) > 0 {
				status := environment.StatusSuccess
				if len(warnings) > 0 {
					status = environment.StatusPartial
				}
				return environment.CollectorResult{
					Name:       c.Name(),
					Status:     status,
					StartedAt:  startedAt,
					FinishedAt: time.Now(),
					Data:       data,
					Devices:    toDevices(devices),
					Warnings:   warnings,
				}
			}
		} else {
			warnings = append(warnings, "lspci failed: "+runErr.Error())
		}
	} else {
		warnings = append(warnings, "lspci not found on PATH")
	}

	// No GPUs detected.
	data["gpu.count"] = "0"
	data["gpu.vendor"] = "unknown"

	status := environment.StatusSuccess
	if len(warnings) > 0 {
		status = environment.StatusPartial
	}

	return environment.CollectorResult{
		Name:       c.Name(),
		Status:     status,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Data:       data,
		Warnings:   warnings,
	}
}

// parseROCmSMIDevices parses `rocm-smi --showproductname` output into GPUDevice entries.
//
// Example input lines:
//
//	GPU[0] : Card Series: AMD Instinct MI300X
func parseROCmSMIDevices(output string) []environment.GPUDevice {
	var devices []environment.GPUDevice
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "Card Series:") {
			continue
		}

		// Extract GPU index from "GPU[0] : Card Series: ..."
		index := len(devices)
		if bracketEnd := strings.Index(line, "]"); bracketEnd > 0 {
			bracketStart := strings.Index(line, "[")
			if bracketStart >= 0 && bracketStart < bracketEnd {
				if n, err := strconv.Atoi(line[bracketStart+1 : bracketEnd]); err == nil {
					index = n
				}
			}
		}

		parts := strings.SplitN(line, "Card Series:", 2)
		if len(parts) != 2 {
			continue
		}
		model := strings.TrimSpace(parts[1])
		if model == "" {
			continue
		}

		devices = append(devices, environment.GPUDevice{
			Index:  index,
			Model:  model,
			Vendor: "AMD",
		})
	}
	return devices
}

// parseLspciDevices filters lspci lines for GPU entries and returns GPUDevice entries.
// The PCI bus ID is extracted from the first field of each lspci line.
func parseLspciDevices(output string) []environment.GPUDevice {
	var devices []environment.GPUDevice
	for _, line := range strings.Split(output, "\n") {
		upper := strings.ToUpper(line)
		if !strings.Contains(upper, "VGA") &&
			!strings.Contains(upper, "3D CONTROLLER") &&
			!strings.Contains(upper, "DISPLAY CONTROLLER") {
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// lspci format: "0000:03:00.0 VGA compatible controller: ..."
		pciID := ""
		description := line
		if spaceIdx := strings.Index(line, " "); spaceIdx > 0 {
			pciID = line[:spaceIdx]
			description = strings.TrimSpace(line[spaceIdx+1:])
		}

		vendor := inferVendorFromLine(upper)

		devices = append(devices, environment.GPUDevice{
			Index:  len(devices),
			Model:  description,
			Vendor: vendor,
			ID:     pciID,
		})
	}
	return devices
}

// inferVendorFromLine deduces the GPU vendor from an uppercased lspci description.
// Uses specific brand markers to avoid false positives (e.g. "COMPATIBLE" contains "ATI").
func inferVendorFromLine(upper string) string {
	switch {
	case strings.Contains(upper, "ADVANCED MICRO DEVICES") ||
		strings.Contains(upper, "[AMD") ||
		strings.Contains(upper, "AMD/ATI"):
		return "AMD"
	case strings.Contains(upper, "NVIDIA"):
		return "NVIDIA"
	default:
		return "unknown"
	}
}

// populateGPUSummary writes summary gpu.* keys into data from the device list.
// Per-device detail lives in CollectorResult.Devices and is not repeated here.
func populateGPUSummary(devices []environment.GPUDevice, data map[string]string) {
	if len(devices) == 0 {
		data["gpu.count"] = "0"
		data["gpu.vendor"] = "unknown"
		return
	}

	data["gpu.count"] = strconv.Itoa(len(devices))

	// Use the first device's vendor as the summary vendor.
	vendor := devices[0].Vendor
	data["gpu.vendor"] = vendor
}

// toDevices wraps a GPUDevice slice in an environment.Devices pointer.
// Returns nil when the slice is empty.
func toDevices(gpus []environment.GPUDevice) *environment.Devices {
	if len(gpus) == 0 {
		return nil
	}
	return &environment.Devices{GPUs: gpus}
}
