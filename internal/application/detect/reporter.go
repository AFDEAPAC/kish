package detect

import (
	"fmt"

	"github.com/AFDEAPAC/kish/internal/domain/environment"
)

// Reporter receives progress events during the detection workflow.
//
// Implementations must be safe to call from the detection loop. They should
// not block or panic. Errors within a reporter must not propagate to the caller.
//
// The CLI layer calls SnapshotWritten after the file has been written because
// only the CLI knows the resolved output path.
type Reporter interface {
	// CollectorStarted is called immediately before a collector runs.
	CollectorStarted(name string)

	// CollectorFinished is called immediately after a collector returns.
	CollectorFinished(name string, status environment.CollectorStatus)

	// SnapshotWritten is called after the snapshot file has been written
	// successfully. path is the resolved output file path.
	SnapshotWritten(path string)
}

// StdoutReporter prints [detect]-prefixed progress messages to stdout.
type StdoutReporter struct{}

// CollectorStarted preserves the CLI's human-readable [detect] progress format.
func (r *StdoutReporter) CollectorStarted(name string) {
	fmt.Printf("[detect] running collector: %s\n", name)
}

// CollectorFinished mirrors collector completion without surfacing reporter errors.
func (r *StdoutReporter) CollectorFinished(name string, status environment.CollectorStatus) {
	fmt.Printf("[detect] %s: %s\n", name, status)
}

// SnapshotWritten reports the resolved path after the CLI has completed the write.
func (r *StdoutReporter) SnapshotWritten(path string) {
	fmt.Printf("[detect] written snapshot: %s\n", path)
}

// NoopReporter discards all progress events.
// Used when the --quiet flag is set.
type NoopReporter struct{}

func (r *NoopReporter) CollectorStarted(_ string)                                 {}
func (r *NoopReporter) CollectorFinished(_ string, _ environment.CollectorStatus) {}
func (r *NoopReporter) SnapshotWritten(_ string)                                  {}
