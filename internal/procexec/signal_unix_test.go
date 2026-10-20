//go:build !windows

package procexec

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// startHelper spawns the test binary in a helper mode (see TestMain in
// procexec_test.go) with forward, and waits until it is ready. The returned
// function waits for the exit code.
func startHelper(t *testing.T, mode string, forward func(*os.Process, os.Signal)) func() int {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	env := []string{"PROCEXEC_HELPER=" + mode, "PROCEXEC_OUT=" + ready}

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := spawn(context.Background(), []string{exe}, env, forward)
		done <- result{code, err}
	}()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper never became ready")
		}
	}
	return func() int {
		r := <-done
		if r.err != nil {
			t.Fatalf("spawn: %v", r.err)
		}
		return r.code
	}
}

func signalSelf(t *testing.T, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		t.Fatal(err)
	}
}

func noTerminal() (int, bool) { return 0, false }

func TestSpawnForwardsInterrupt(t *testing.T) {
	wait := startHelper(t, "wait-interrupt", forwarderWith(noTerminal))
	signalSelf(t, syscall.SIGINT)
	if code := wait(); code != 42 {
		t.Fatalf("code = %d, want 42 (child handled SIGINT)", code)
	}
}

func TestSpawnReportsSignalDeath(t *testing.T) {
	wait := startHelper(t, "wait", forwarderWith(noTerminal))
	signalSelf(t, syscall.SIGTERM)
	if code := wait(); code != 128+int(syscall.SIGTERM) {
		t.Fatalf("code = %d, want %d", code, 128+int(syscall.SIGTERM))
	}
}

func TestSpawnSkipsInterruptForForegroundTerminal(t *testing.T) {
	// The terminal reports our group in the foreground, so it already sent
	// SIGINT to the child; secretref must not send it again. The lookup runs
	// once per signal.
	var lookups atomic.Int32
	foreground := func() (int, bool) {
		lookups.Add(1)
		return syscall.Getpgrp(), true
	}
	wait := startHelper(t, "wait-interrupt", forwarderWith(foreground))
	signalSelf(t, syscall.SIGINT)
	for deadline := time.Now().Add(5 * time.Second); lookups.Load() < 1; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("SIGINT never reached the forwarder")
		}
	}
	signalSelf(t, syscall.SIGTERM) // always forwarded; the helper dies from it
	if code := wait(); code != 128+int(syscall.SIGTERM) {
		t.Fatalf("code = %d, want %d: SIGINT was forwarded or SIGTERM was not", code, 128+int(syscall.SIGTERM))
	}
	if n := lookups.Load(); n != 2 {
		t.Errorf("terminal looked up %d times, want once per signal (2)", n)
	}
}

func TestControllingTTYForeground(t *testing.T) {
	// Agrees with whether /dev/tty can be opened: false in CI, true in a
	// terminal, regardless of where stdin points.
	hasTTY := false
	if f, err := os.Open("/dev/tty"); err == nil {
		hasTTY = true
		_ = f.Close()
	}
	if _, ok := controllingTTYForeground(); ok != hasTTY {
		t.Errorf("controllingTTYForeground ok = %v, want %v", ok, hasTTY)
	}
}
