package procexec

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// ErrCommandNotFound reports that the command is not on the child's PATH or is
// not executable.
var ErrCommandNotFound = errors.New("command not found")

// ErrReplaceUnsupported reports that the OS cannot replace the current
// process; callers use Spawn instead.
var ErrReplaceUnsupported = errors.New("replacing the process is not supported on this OS")

var errNoCommand = errors.New("no command given")

// Spawn runs argv with exactly env as its environment and the parent's
// standard streams. It catches interrupt, quit, hangup, and termination
// signals, passes on those the child would not otherwise receive, and returns
// the child's exit code. A child killed by a signal yields 128 plus the signal
// number.
func Spawn(ctx context.Context, argv, env []string) (int, error) {
	return spawn(ctx, argv, env, newForwarder())
}

// spawn is Spawn with the signal forwarder injected, for tests.
func spawn(ctx context.Context, argv, env []string, forward func(*os.Process, os.Signal)) (int, error) {
	path, err := lookPath(argv, env)
	if err != nil {
		return 0, err
	}
	//nolint:gosec // G204: running the user's command is the purpose of secretref.
	cmd := exec.CommandContext(ctx, path)
	cmd.Args = argv
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, caughtSignals()...)
	defer signal.Stop(sigs)

	if err := cmd.Start(); err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return 0, fmt.Errorf("%s: %w", argv[0], ErrCommandNotFound)
		}
		return 0, fmt.Errorf("start %s: %w", argv[0], err)
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigs:
				forward(cmd.Process, s)
			case <-done:
				return
			}
		}
	}()
	err = cmd.Wait()
	close(done)

	if cmd.ProcessState == nil {
		return 0, fmt.Errorf("wait %s: %w", argv[0], err)
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return 0, fmt.Errorf("wait %s: %w", argv[0], err)
	}
	return exitStatus(cmd.ProcessState), nil
}

// Hold catches the signals Spawn handles until stop is called, so that a
// signal arriving before the child starts does not kill this process and skip
// its cleanup. received reports the first signal caught so far, if any.
func Hold() (received func() (os.Signal, bool), stop func()) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, caughtSignals()...)
	var first os.Signal
	received = func() (os.Signal, bool) {
		if first == nil {
			select {
			case first = <-sigs:
			default:
			}
		}
		return first, first != nil
	}
	return received, func() { signal.Stop(sigs) }
}

// SignalExitCode returns the shell-style exit code for a process ended by s:
// 128 plus the signal number.
func SignalExitCode(s os.Signal) int {
	if n, ok := s.(syscall.Signal); ok {
		return 128 + int(n)
	}
	return 128 + int(syscall.SIGINT)
}

// lookPath finds argv[0] the way the child would see it: a name containing a
// path separator is used as given; a bare name is searched in the PATH from
// env, not in the parent's PATH.
func lookPath(argv, env []string) (string, error) {
	if len(argv) == 0 || argv[0] == "" {
		return "", errNoCommand
	}
	name := argv[0]
	notFound := fmt.Errorf("%s: %w", name, ErrCommandNotFound)
	if strings.ContainsAny(name, separators()) {
		p, err := exec.LookPath(name)
		if err != nil {
			return "", notFound
		}
		return p, nil
	}
	for _, dir := range filepath.SplitList(envValue(env, "PATH")) {
		// SECURITY: empty and relative PATH entries are skipped so that a bare
		// name never runs a file from the working directory by accident.
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		if p, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return p, nil
		}
	}
	return "", notFound
}

func separators() string {
	if runtime.GOOS == "windows" {
		return `/\:`
	}
	return "/"
}

// envValue returns the last value of key in env. Names compare
// case-insensitively on Windows.
func envValue(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		k, v, ok := strings.Cut(env[i], "=")
		if !ok {
			continue
		}
		if k == key || (runtime.GOOS == "windows" && strings.EqualFold(k, key)) {
			return v
		}
	}
	return ""
}
