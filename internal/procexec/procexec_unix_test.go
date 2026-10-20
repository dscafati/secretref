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

func TestNonExecutableIsNotFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plain"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := procexec.Spawn(context.Background(), []string{"plain"}, []string{"PATH=" + dir})
	if !errors.Is(err, procexec.ErrCommandNotFound) {
		t.Fatalf("error = %v, want ErrCommandNotFound", err)
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
