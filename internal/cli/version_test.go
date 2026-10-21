package cli_test

import (
	"context"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/dscafati/secretref/internal/cli"
)

func TestVersionFlag(t *testing.T) {
	h := newHarness(t)
	h.expect(0, "version")
	want := h.stdout.String()
	h.expect(0, "--version")
	if got := h.stdout.String(); got != want {
		t.Errorf("--version stdout = %q, want %q (same as version)", got, want)
	}
	if h.stderr.Len() != 0 {
		t.Errorf("stderr = %q", h.stderr.String())
	}
}

func TestVersionFlagInHelp(t *testing.T) {
	h := newHarness(t)
	h.expect(0, "--help")
	if !strings.Contains(h.stdout.String(), "--version") {
		t.Errorf("help does not list --version:\n%s", h.stdout.String())
	}
}

func TestVersionFromBuildInfo(t *testing.T) {
	h := newHarness(t)
	d := h.deps()
	d.ReadBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/dscafati/secretref", Version: "v0.1.0"}}, true
	}
	for _, args := range [][]string{{"version"}, {"--version"}} {
		h.stdout.Reset()
		if code := cli.Execute(context.Background(), args, d); code != 0 {
			t.Fatalf("secretref %q = %d, stderr:\n%s", args, code, h.stderr.String())
		}
		if got, want := h.stdout.String(), "secretref v0.1.0\n"; got != want {
			t.Errorf("secretref %q stdout = %q, want %q", args, got, want)
		}
	}
}
