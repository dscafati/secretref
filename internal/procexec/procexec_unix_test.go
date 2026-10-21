//go:build !windows

package procexec_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/dscafati/secretref/internal/procexec"
)

func TestReplaceExecs(t *testing.T) {
	// The helper calls Replace, which execs the helper again in mode exit9.
	code, err := procexec.Spawn(context.Background(), []string{self(t)}, []string{helperEnv + "=replace"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 9 {
		t.Fatalf("code = %d, want 9", code)
	}
}

func TestLookupUsesChildPath(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "secretref-test-tool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 4\n"), 0o700); err != nil { //nolint:gosec // test script must be executable
		t.Fatal(err)
	}
	code, err := procexec.Spawn(context.Background(), []string{"secretref-test-tool"}, []string{"PATH=" + dir})
	if err != nil {
		t.Fatal(err)
	}
	if code != 4 {
		t.Fatalf("code = %d, want 4", code)
	}
}

func TestHoldCatchesSignal(t *testing.T) {
	received, stop := procexec.Hold()
	defer stop()
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s, ok := received(); ok {
			if s != syscall.SIGHUP {
				t.Fatalf("received %v, want SIGHUP", s)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no signal received")
}

func TestCommandNotExecutable(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	noShebang := filepath.Join(dir, "noshebang")
	if err := os.WriteFile(noShebang, []byte("exit 0\n"), 0o700); err != nil { //nolint:gosec // test script must be executable
		t.Fatal(err)
	}
	tests := map[string][]string{
		"not executable": {plain},
		"directory":      {dir},
		"on PATH":        {"plain"},
	}
	for name, argv := range tests {
		t.Run(name, func(t *testing.T) {
			env := []string{"PATH=" + dir}
			if _, err := procexec.Spawn(context.Background(), argv, env); !errors.Is(err, procexec.ErrCommandNotExecutable) {
				t.Errorf("Spawn error = %v, want ErrCommandNotExecutable", err)
			}
			if err := procexec.Replace(argv, env); !errors.Is(err, procexec.ErrCommandNotExecutable) {
				t.Errorf("Replace error = %v, want ErrCommandNotExecutable", err)
			}
		})
	}
	t.Run("no shebang", func(t *testing.T) {
		argv := []string{noShebang}
		if err := procexec.Replace(argv, []string{"PATH=" + dir}); !errors.Is(err, procexec.ErrCommandNotExecutable) {
			t.Errorf("Replace error = %v, want ErrCommandNotExecutable", err)
		}
	})
}
