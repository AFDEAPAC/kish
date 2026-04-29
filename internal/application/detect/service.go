// Package detect provides the application-layer detection service.
//
// The service coordinates the collector registry, runs all collectors,
// tolerates individual failures, and merges results into an EnvironmentSnapshot.
// It belongs in the application layer because it orchestrates the detection
// workflow without being tied to any specific infrastructure detail.
package detect

import (
	"context"
	"time"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
)

// KishVersion is the current version of the kish CLI.
// It is embedded in every produced snapshot.
const KishVersion = "0.1.0"

// DetectionService runs all registered collectors and assembles an
// EnvironmentSnapshot from their combined results.
type DetectionService struct {
	collectors []environment.Collector
}

// NewDetectionService constructs a DetectionService with the provided collectors.
func NewDetectionService(collectors []environment.Collector) *DetectionService {
	return &DetectionService{collectors: collectors}
}

// Detect runs all collectors, merges their results, and returns an
// EnvironmentSnapshot. Individual collector failures are tolerated; the
// returned error is non-nil only for critical setup or serialization failures.
//
// The reporter receives progress events around each collector run.
// Pass a NoopReporter to suppress all progress output.
func (s *DetectionService) Detect(ctx context.Context, reporter Reporter) (*environment.EnvironmentSnapshot, error) {
	collectedAt := time.Now().UTC()

	mergedData := make(map[string]string)
	mergedRaw := make(map[string]string)
	var mergedPackageSets []environment.PackageSet
	var reports []environment.CollectorReport
	var mergedDevices *environment.Devices

	for _, c := range s.collectors {
		reporter.CollectorStarted(c.Name())
		result := c.Collect(ctx)
		reporter.CollectorFinished(c.Name(), result.Status)

		// Merge data; later collectors override earlier ones for duplicate keys.
		for k, v := range result.Data {
			mergedData[k] = v
		}

		// Merge raw; package-level raw outputs are written to separate files by collectors.
		for k, v := range result.Raw {
			mergedRaw[k] = v
		}

		// Merge package sets; collectors set RawFile to reference external raw output files.
		mergedPackageSets = append(mergedPackageSets, result.PackageSets...)

		// Merge structured device data.
		if result.Devices != nil {
			if mergedDevices == nil {
				mergedDevices = &environment.Devices{}
			}
			mergedDevices.GPUs = append(mergedDevices.GPUs, result.Devices.GPUs...)
		}

		reports = append(reports, environment.CollectorReport{
			Name:     result.Name,
			Status:   result.Status,
			Warnings: result.Warnings,
			Error:    result.Error,
		})
	}

	envType := inferEnvironmentType(mergedData)

	// Ensure package_sets and collectors are never null in JSON output.
	if mergedPackageSets == nil {
		mergedPackageSets = []environment.PackageSet{}
	}
	if reports == nil {
		reports = []environment.CollectorReport{}
	}

	snapshot := &environment.EnvironmentSnapshot{
		SchemaVersion: environment.SchemaVersionV1,
		KishVersion:   KishVersion,
		Scope:         environment.ScopeExecution,
		Type:          envType,
		CollectedAt:   collectedAt,
		Data:          mergedData,
		PackageSets:   mergedPackageSets,
		Raw:           mergedRaw,
		Collectors:    reports,
		Devices:       mergedDevices,
	}

	return snapshot, nil
}

// inferEnvironmentType determines the environment type from the merged data map.
// The ContainerCollector sets "container.detected" to "true" when /.dockerenv exists.
func inferEnvironmentType(data map[string]string) environment.EnvironmentType {
	if data["container.detected"] == "true" {
		return environment.TypeContainer
	}
	return environment.TypeHost
}
