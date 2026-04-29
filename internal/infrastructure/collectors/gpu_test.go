package collectors_test

import (
	"context"
	"testing"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/collectors"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
	"github.com/AFDEAPAC/kish/internal/testutil"
)

func TestGPUCollector_NoGPUTools_SuccessWithZeroCount(t *testing.T) {
	runner := testutil.NewFakeRunner()
	// Neither rocm-smi nor lspci are found (default FakeRunner behaviour).

	c := collectors.NewGPUCollector(runner)
	result := c.Collect(context.Background())

	// No GPU tools should not cause a hard failure.
	if result.Status == environment.StatusFailed {
		t.Errorf("expected non-failed status, got %q", result.Status)
	}
	assertData(t, result, "gpu.count", "0")

	// Devices should be nil when no GPUs are detected.
	if result.Devices != nil && len(result.Devices.GPUs) != 0 {
		t.Errorf("expected no GPU devices, got %+v", result.Devices)
	}
}

func TestGPUCollector_LspciWithAMD_VendorIsAMD(t *testing.T) {
	lspciOutput := `0000:03:00.0 VGA compatible controller: Advanced Micro Devices, Inc. [AMD/ATI] (rev c8)
0000:04:00.0 3D controller: Advanced Micro Devices, Inc. [AMD/ATI] Instinct MI300X (rev c8)
`
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["lspci"] = nil
	runner.RunResponses["lspci"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: lspciOutput},
	}

	c := collectors.NewGPUCollector(runner)
	result := c.Collect(context.Background())

	assertData(t, result, "gpu.vendor", "AMD")
	assertData(t, result, "gpu.count", "2")

	// gpu.models must not exist as a comma-separated string.
	if _, ok := result.Data["gpu.models"]; ok {
		t.Error("gpu.models should not be set; use devices.gpus instead")
	}

	// Structured devices should be populated.
	if result.Devices == nil {
		t.Fatal("expected Devices to be populated")
	}
	if len(result.Devices.GPUs) != 2 {
		t.Errorf("expected 2 GPU devices, got %d", len(result.Devices.GPUs))
	}
	if result.Devices.GPUs[0].Vendor != "AMD" {
		t.Errorf("expected vendor=AMD, got %q", result.Devices.GPUs[0].Vendor)
	}
	// PCI ID should be extracted from the first field of each lspci line.
	if result.Devices.GPUs[0].ID != "0000:03:00.0" {
		t.Errorf("expected ID=0000:03:00.0, got %q", result.Devices.GPUs[0].ID)
	}
}

func TestGPUCollector_LspciWithNVIDIA_VendorIsNVIDIA(t *testing.T) {
	lspciOutput := `0000:01:00.0 VGA compatible controller: NVIDIA Corporation GA102 [GeForce RTX 3090] (rev a1)
`
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["lspci"] = nil
	runner.RunResponses["lspci"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: lspciOutput},
	}

	c := collectors.NewGPUCollector(runner)
	result := c.Collect(context.Background())

	assertData(t, result, "gpu.vendor", "NVIDIA")
	assertData(t, result, "gpu.count", "1")

	if result.Devices == nil || len(result.Devices.GPUs) != 1 {
		t.Fatal("expected 1 structured GPU device")
	}
	if result.Devices.GPUs[0].Vendor != "NVIDIA" {
		t.Errorf("expected vendor=NVIDIA, got %q", result.Devices.GPUs[0].Vendor)
	}
}

func TestGPUCollector_RepeatedSameModel_ProducesStructuredDevices(t *testing.T) {
	// Simulate 4 identical AMD GPUs — the problematic case that previously
	// produced a long comma-separated gpu.models string.
	lspciOutput := `0000:01:00.0 3D controller: Advanced Micro Devices, Inc. [AMD/ATI] Instinct MI308X
0000:02:00.0 3D controller: Advanced Micro Devices, Inc. [AMD/ATI] Instinct MI308X
0000:03:00.0 3D controller: Advanced Micro Devices, Inc. [AMD/ATI] Instinct MI308X
0000:04:00.0 3D controller: Advanced Micro Devices, Inc. [AMD/ATI] Instinct MI308X
`
	runner := testutil.NewFakeRunner()
	runner.LookPathResponses["lspci"] = nil
	runner.RunResponses["lspci"] = testutil.FakeRunResponse{
		Result: command.CommandResult{Stdout: lspciOutput},
	}

	c := collectors.NewGPUCollector(runner)
	result := c.Collect(context.Background())

	assertData(t, result, "gpu.count", "4")
	if _, ok := result.Data["gpu.models"]; ok {
		t.Error("gpu.models must not be emitted; use devices.gpus")
	}
	if result.Devices == nil || len(result.Devices.GPUs) != 4 {
		t.Fatalf("expected 4 structured GPU devices, got %v", result.Devices)
	}
	// Each device should have an index.
	for i, dev := range result.Devices.GPUs {
		if dev.Index != i {
			t.Errorf("device[%d] has wrong index %d", i, dev.Index)
		}
	}
}
