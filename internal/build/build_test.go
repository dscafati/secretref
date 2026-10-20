package build_test

import (
	"testing"

	"github.com/dscafati/secretref/internal/build"
)

func TestVersionDefault(t *testing.T) {
	if build.Version != "dev" {
		t.Fatalf("Version = %q, want %q", build.Version, "dev")
	}
}
