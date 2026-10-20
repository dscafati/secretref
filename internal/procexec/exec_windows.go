//go:build windows

package procexec

import (
	"os"
)

// Replace returns ErrReplaceUnsupported: Windows cannot replace a process.
// Callers use Spawn.
func Replace([]string, []string) error {
	return ErrReplaceUnsupported
}

func caughtSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

// newForwarder returns a forwarder that does nothing: the console sends
// Ctrl+C to the child itself. The parent only catches it so that it lives
// until the child exits.
func newForwarder() func(*os.Process, os.Signal) {
	return func(*os.Process, os.Signal) {}
}

func exitStatus(state *os.ProcessState) int {
	return state.ExitCode()
}
