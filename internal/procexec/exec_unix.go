//go:build !windows

package procexec

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Replace runs argv in place of the current process with exactly env as its
// environment. It returns only on failure.
func Replace(argv, env []string) error {
	path, err := lookPath(argv, env)
	if err != nil {
		return err
	}
	//nolint:gosec // G204: running the user's command is the purpose of secretref.
	err = syscall.Exec(path, argv, env)
	if errors.Is(err, syscall.ENOENT) {
		return fmt.Errorf("%s: %w", argv[0], ErrCommandNotFound)
	}
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.ENOEXEC) {
		return fmt.Errorf("%s: %w", argv[0], ErrCommandNotExecutable)
	}
	return fmt.Errorf("exec %s: %w", argv[0], err)
}

func caughtSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}
}

// newForwarder returns the forwarder Spawn uses: it consults the controlling
// terminal.
func newForwarder() func(*os.Process, os.Signal) {
	return forwarderWith(controllingTTYForeground)
}

// forwarderWith returns a function that passes a signal to the child unless
// shouldForward says the terminal already delivered it. The terminal is
// Known limitation: a SIGINT sent directly to secretref's PID while it is in
// the terminal's foreground group is not forwarded, because it cannot be told
// apart from the terminal's own Ctrl+C, which already reached the child.
// The terminal is
// looked up for every signal, because job control (Ctrl+Z, bg, fg) moves
// secretref in and out of the foreground while the child runs.
func forwarderWith(foreground func() (int, bool)) func(*os.Process, os.Signal) {
	return func(p *os.Process, s os.Signal) {
		fg, isTTY := foreground()
		if shouldForward(s, isTTY, fg, syscall.Getpgrp()) {
			_ = p.Signal(s)
		}
	}
}

// controllingTTYForeground returns the foreground process group of the
// controlling terminal. It opens /dev/tty rather than using stdin, so it also
// works when stdin is redirected. ok is false without a controlling terminal.
func controllingTTYForeground() (int, bool) {
	fd, err := unix.Open("/dev/tty", unix.O_RDONLY|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, false
	}
	defer func() { _ = unix.Close(fd) }()
	fg, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return 0, false
	}
	return fg, true
}

// shouldForward reports whether sig goes on to the child. When the
// controlling terminal's foreground process group is ours, the child is in that group
// too, and the terminal already delivered SIGINT, SIGQUIT, and SIGHUP to it.
// Forwarding them again would deliver them twice, which makes docker compose
// force-stop its containers. SIGTERM never comes from the terminal and is
// always forwarded.
func shouldForward(sig os.Signal, isTTY bool, foreground, own int) bool {
	if sig == syscall.SIGTERM {
		return true
	}
	return !isTTY || foreground != own
}

// exitStatus returns the exit code, or 128 plus the signal number for a child
// killed by a signal, as shells do.
func exitStatus(state *os.ProcessState) int {
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return state.ExitCode()
}
