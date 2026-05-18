package cli

import (
	"github.com/AFDEAPAC/kish/internal/domain/environment"
	"github.com/AFDEAPAC/kish/internal/infrastructure/collectors"
	"github.com/AFDEAPAC/kish/internal/infrastructure/command"
)

// defaultDetectCollectors returns the ordered collector list used by the
// `kish detect` command.
//
// The composition lives in the CLI adapter because it chooses concrete
// infrastructure collectors. The detect application service only coordinates
// the collectors it receives.
func defaultDetectCollectors(runner command.Runner, outputDir string) []environment.Collector {
	return []environment.Collector{
		collectors.NewLinuxSystemCollector(runner),
		collectors.NewContainerCollector(runner),
		collectors.NewSystemPackageCollector(runner, outputDir),
		collectors.NewROCmCollector(runner),
		collectors.NewGPUCollector(runner),
		collectors.NewPythonPackageCollector(runner, outputDir),
	}
}
