package procexec_test

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dscafati/secretref/internal/procexec"
)

// helperEnv selects the helper mode in TestMain when the test binary runs
// itself as the child process.
const helperEnv = "PROCEXEC_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(runHelper(mode))
	}
	os.Exit(m.Run())
}

func runHelper(mode string) int {
	switch mode {
	case "exit7":
		return 7
	case "exit9":
		return 9
	case "env":
		data := strings.Join(os.Environ(), "\n")
		//nolint:gosec // G703: the test sets PROCEXEC_OUT to a path in its temp dir.
		if err := os.WriteFile(os.Getenv("PROCEXEC_OUT"), []byte(data), 0o600); err != nil {
			return 98
		}
		return 0
	case "replace":
		env := []string{helperEnv + "=exit9"}
		if err := procexec.Replace([]string{os.Args[0]}, env); err != nil {
			return 99
		}
		return 97 // Replace must not return on success.
	case "wait-interrupt":
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, os.Interrupt)
		if !markReady() {
			return 98
		}
		select {
		case <-sigs:
			return 42
		case <-time.After(10 * time.Second):
			return 96
		}
	case "wait":
		if !markReady() {
			return 98
		}
		time.Sleep(10 * time.Second)
		return 96
	}
	return 95
}

// markReady tells the test that the helper installed its signal handlers.
func markReady() bool {
	//nolint:gosec // G703: the test sets PROCEXEC_OUT to a path in its temp dir.
	return os.WriteFile(os.Getenv("PROCEXEC_OUT"), []byte("ready"), 0o600) == nil
}

func self(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestSpawnReturnsChildExitCode(t *testing.T) {
	code, err := procexec.Spawn(context.Background(), []string{self(t)}, []string{helperEnv + "=exit7"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if code != 7 {
		t.Fatalf("code = %d, want 7", code)
	}
}

func TestSpawnPassesEnvExactly(t *testing.T) {
	out := filepath.Join(t.TempDir(), "env.txt")
	env := []string{helperEnv + "=env", "PROCEXEC_OUT=" + out, "A=1", "B=two words"}
	code, err := procexec.Spawn(context.Background(), []string{self(t)}, env)
	if err != nil || code != 0 {
		t.Fatalf("Spawn = %d, %v", code, err)
	}
	data, err := os.ReadFile(out) //nolint:gosec // G304: a file in the test temp dir.
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(string(data), "\n")
	if runtime.GOOS == "windows" {
		// os/exec adds SYSTEMROOT on Windows; check ours are all present.
		for _, kv := range env {
			if !slices.Contains(got, kv) {
				t.Errorf("child env lacks %q: %v", kv, got)
			}
		}
		return
	}
	if diff := cmp.Diff(env, got); diff != "" {
		t.Errorf("child env (-want +got):\n%s", diff)
	}
}

func TestCommandNotFound(t *testing.T) {
	empty := t.TempDir()
	tests := map[string]string{
		"bare name":      "secretref-no-such-command",
		"path separator": filepath.Join(empty, "missing"),
	}
	for name, cmd := range tests {
		t.Run(name, func(t *testing.T) {
			env := []string{"PATH=" + empty}
			_, err := procexec.Spawn(context.Background(), []string{cmd}, env)
			if !errors.Is(err, procexec.ErrCommandNotFound) {
				t.Errorf("Spawn error = %v, want ErrCommandNotFound", err)
			}
			// Windows cannot replace a process: Replace returns
			// ErrReplaceUnsupported there before it looks up the command.
			if runtime.GOOS == "windows" {
				return
			}
			if err := procexec.Replace([]string{cmd}, env); !errors.Is(err, procexec.ErrCommandNotFound) {
				t.Errorf("Replace error = %v, want ErrCommandNotFound", err)
			}
		})
	}
}

func TestCommandLookupIgnoresParentPath(t *testing.T) {
	// "go" is on the parent's PATH while tests run; the child's PATH is empty.
	_, err := procexec.Spawn(context.Background(), []string{"go", "version"}, []string{"PATH="})
	if !errors.Is(err, procexec.ErrCommandNotFound) {
		t.Fatalf("Spawn error = %v, want ErrCommandNotFound", err)
	}
}

func TestEmptyArgv(t *testing.T) {
	if _, err := procexec.Spawn(context.Background(), nil, nil); err == nil {
		t.Error("Spawn(nil) succeeded")
	}
	if err := procexec.Replace(nil, nil); err == nil {
		t.Error("Replace(nil) succeeded")
	}
}

func TestSignalExitCode(t *testing.T) {
	if got := procexec.SignalExitCode(syscall.SIGTERM); got != 128+int(syscall.SIGTERM) {
		t.Errorf("SignalExitCode(SIGTERM) = %d", got)
	}
}

func TestHoldWithoutSignal(t *testing.T) {
	received, stop := procexec.Hold()
	defer stop()
	if s, ok := received(); ok {
		t.Errorf("received %v without a signal", s)
	}
}
