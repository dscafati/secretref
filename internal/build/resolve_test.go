package build

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	info := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Path: "github.com/dscafati/secretref", Version: v}}, true
		}
	}
	none := func() (*debug.BuildInfo, bool) { return nil, false }
	tests := map[string]struct {
		linked string
		read   func() (*debug.BuildInfo, bool)
		want   string
	}{
		"ldflags win":        {"0.1.0", info("v0.2.0"), "0.1.0"},
		"go install":         {"dev", info("v0.1.0"), "v0.1.0"},
		"devel build":        {"dev", info("(devel)"), "dev"},
		"empty module":       {"dev", info(""), "dev"},
		"no build info":      {"dev", none, "dev"},
		"nil reader":         {"dev", nil, "dev"},
		"ldflags, no reader": {"0.1.0", nil, "0.1.0"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := resolve(tt.linked, tt.read); got != tt.want {
				t.Errorf("resolve(%q) = %q, want %q", tt.linked, got, tt.want)
			}
		})
	}
}

func TestResolveUsesVersion(t *testing.T) {
	if got := Resolve(nil); got != Version {
		t.Errorf("Resolve(nil) = %q, want %q", got, Version)
	}
}
