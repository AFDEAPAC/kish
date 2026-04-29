package detect_test

import (
	"context"
	"testing"
	"time"

	"github.com/AFDEAPAC/kish/internal/application/detect"
	"github.com/AFDEAPAC/kish/internal/domain/environment"
)

// noop is a shared no-op reporter used across tests.
var noop detect.Reporter = &detect.NoopReporter{}

// failingCollector always returns StatusFailed to verify that single collector
// failures do not abort the detection service.
type failingCollector struct{}

func (f *failingCollector) Name() string { return "always_fail" }
func (f *failingCollector) Collect(_ context.Context) environment.CollectorResult {
	return environment.CollectorResult{
		Name:       f.Name(),
		Status:     environment.StatusFailed,
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
		Error:      "simulated failure",
	}
}

// dataCollector injects a fixed data map and optional package sets into the snapshot.
type dataCollector struct {
	name    string
	data    map[string]string
	sets    []environment.PackageSet
	devices *environment.Devices
}

func (d *dataCollector) Name() string { return d.name }
func (d *dataCollector) Collect(_ context.Context) environment.CollectorResult {
	return environment.CollectorResult{
		Name:        d.Name(),
		Status:      environment.StatusSuccess,
		StartedAt:   time.Now(),
		FinishedAt:  time.Now(),
		Data:        d.data,
		PackageSets: d.sets,
		Devices:     d.devices,
	}
}

func TestDetect_ContainerDetectedTrue_TypeIsContainer(t *testing.T) {
	cols := []environment.Collector{
		&dataCollector{
			name: "fake_container",
			data: map[string]string{"container.detected": "true"},
		},
	}
	svc := detect.NewDetectionService(cols)
	snap, err := svc.Detect(context.Background(), noop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Type != environment.TypeContainer {
		t.Errorf("expected type=%q, got %q", environment.TypeContainer, snap.Type)
	}
}

func TestDetect_ContainerDetectedFalse_TypeIsHost(t *testing.T) {
	cols := []environment.Collector{
		&dataCollector{
			name: "fake_container",
			data: map[string]string{"container.detected": "false"},
		},
	}
	svc := detect.NewDetectionService(cols)
	snap, err := svc.Detect(context.Background(), noop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Type != environment.TypeHost {
		t.Errorf("expected type=%q, got %q", environment.TypeHost, snap.Type)
	}
}

func TestDetect_CollectorFailure_DoesNotFail(t *testing.T) {
	cols := []environment.Collector{&failingCollector{}}
	svc := detect.NewDetectionService(cols)
	snap, err := svc.Detect(context.Background(), noop)
	if err != nil {
		t.Fatalf("detection should succeed even when a collector fails, got: %v", err)
	}
	if len(snap.Collectors) != 1 {
		t.Fatalf("expected 1 collector report, got %d", len(snap.Collectors))
	}
	if snap.Collectors[0].Status != environment.StatusFailed {
		t.Errorf("expected collector status=%q, got %q", environment.StatusFailed, snap.Collectors[0].Status)
	}
}

func TestDetect_PackageSetsFromMultipleCollectors_AreAppended(t *testing.T) {
	set1 := environment.PackageSet{Type: "system", Manager: "dpkg"}
	set2 := environment.PackageSet{Type: "python", Manager: "pip"}
	cols := []environment.Collector{
		&dataCollector{name: "c1", data: map[string]string{}, sets: []environment.PackageSet{set1}},
		&dataCollector{name: "c2", data: map[string]string{}, sets: []environment.PackageSet{set2}},
	}
	svc := detect.NewDetectionService(cols)
	snap, err := svc.Detect(context.Background(), noop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snap.PackageSets) != 2 {
		t.Errorf("expected 2 package sets, got %d", len(snap.PackageSets))
	}
}

func TestDetect_DevicesFromCollectors_AreMerged(t *testing.T) {
	gpus1 := &environment.Devices{GPUs: []environment.GPUDevice{{Index: 0, Model: "GPU-A", Vendor: "AMD"}}}
	gpus2 := &environment.Devices{GPUs: []environment.GPUDevice{{Index: 1, Model: "GPU-B", Vendor: "AMD"}}}
	cols := []environment.Collector{
		&dataCollector{name: "c1", data: map[string]string{}, devices: gpus1},
		&dataCollector{name: "c2", data: map[string]string{}, devices: gpus2},
	}
	svc := detect.NewDetectionService(cols)
	snap, err := svc.Detect(context.Background(), noop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Devices == nil {
		t.Fatal("expected Devices to be populated")
	}
	if len(snap.Devices.GPUs) != 2 {
		t.Errorf("expected 2 GPUs merged, got %d", len(snap.Devices.GPUs))
	}
}

func TestDetect_SnapshotFields_ArePopulated(t *testing.T) {
	svc := detect.NewDetectionService(nil)
	snap, err := svc.Detect(context.Background(), noop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.SchemaVersion != environment.SchemaVersionV1 {
		t.Errorf("unexpected schema_version: %q", snap.SchemaVersion)
	}
	if snap.Scope != environment.ScopeExecution {
		t.Errorf("unexpected scope: %q", snap.Scope)
	}
	if snap.KishVersion == "" {
		t.Error("kish_version must not be empty")
	}
	if snap.CollectedAt.IsZero() {
		t.Error("collected_at must not be zero")
	}
}

// reportingCollector records calls to CollectorStarted and CollectorFinished.
type recordingReporter struct {
	started  []string
	finished []string
}

func (r *recordingReporter) CollectorStarted(name string)                              { r.started = append(r.started, name) }
func (r *recordingReporter) CollectorFinished(name string, _ environment.CollectorStatus) { r.finished = append(r.finished, name) }
func (r *recordingReporter) SnapshotWritten(_ string)                                  {}

func TestDetect_Reporter_ReceivesCollectorEvents(t *testing.T) {
	cols := []environment.Collector{
		&dataCollector{name: "alpha", data: map[string]string{}},
		&dataCollector{name: "beta", data: map[string]string{}},
	}
	rec := &recordingReporter{}
	svc := detect.NewDetectionService(cols)
	_, err := svc.Detect(context.Background(), rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.started) != 2 || rec.started[0] != "alpha" || rec.started[1] != "beta" {
		t.Errorf("unexpected started events: %v", rec.started)
	}
	if len(rec.finished) != 2 {
		t.Errorf("unexpected finished events: %v", rec.finished)
	}
}
