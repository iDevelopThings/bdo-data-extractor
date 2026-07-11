// Package progress is the pipeline's progress sink. It holds a single settable
// Reporter (like slog's default logger) so build/extraction stages can report
// without threading a reporter argument through every call. The default prints to
// stdout, preserving the CLI's console output; an embedder swaps in its own sink.
package progress

import "fmt"

// Reporter receives progress from a running pipeline. Step marks the top-level
// command (build, loc, icons, …); Phase marks a sub-stage within one; Progress
// reports per-item counts (total 0 when unknown); Log carries a freeform line.
type Reporter interface {
	Step(index, total int, phase string)
	Phase(name string)
	Progress(done, total int64)
	Log(line string)
}

var current Reporter = Log{}

// Set installs r as the process-wide sink. Callers that run pipelines
// concurrently must serialize themselves — the sink is a single global.
func Set(r Reporter) {
	if r == nil {
		r = Nop{}
	}
	current = r
}

func Default() Reporter {
	return current
}

// Nop discards everything.
type Nop struct{}

func (Nop) Step(int, int, string) {}
func (Nop) Phase(string)          {}
func (Nop) Progress(int64, int64) {}
func (Nop) Log(string)            {}

// Log is the default sink: it prints to stdout the way the CLI always has.
type Log struct{}

func (Log) Step(index, total int, phase string) {
	fmt.Printf("[%d/%d] %s\n", index, total, phase)
}

func (Log) Phase(name string) {
	fmt.Printf("[STAGE -> %s]\n", name)
}

func (Log) Progress(done, total int64) {
	if total > 0 {
		fmt.Printf("  %d/%d\n", done, total)
		return
	}
	fmt.Printf("  %d\n", done)
}

func (Log) Log(line string) {
	fmt.Println(line)
}
