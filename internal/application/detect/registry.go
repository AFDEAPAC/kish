package detect

import (
	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/collectors"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

// DefaultCollectors returns the ordered list of v1 collectors.
//
// Ordering is intentional: general system facts are collected first so that
// later collectors (e.g. gpu) have a predictable base to build on. Collectors
// must not depend on each other's data at runtime, but ordering provides a
// logical narrative in the output snapshot.
//
// outputDir is the directory where package collectors write their raw output
// files. Pass "." or the directory of the snapshot JSON file.
func DefaultCollectors(runner command.Runner, outputDir string) []environment.Collector {
	return []environment.Collector{
		collectors.NewLinuxSystemCollector(runner),
		collectors.NewContainerCollector(runner),
		collectors.NewSystemPackageCollector(runner, outputDir),
		collectors.NewROCmCollector(runner),
		collectors.NewGPUCollector(runner),
		collectors.NewPythonPackageCollector(runner, outputDir),
	}
}
