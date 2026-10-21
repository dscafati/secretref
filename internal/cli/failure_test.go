package cli

import (
	"strings"
	"testing"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/resolve"
)

// A reference that somehow holds a control character is printed escaped, so
// it cannot drive the terminal. ref.Parse already rejects such references;
// this is the second line of defense.
func TestFailureLineEscapesReference(t *testing.T) {
	f := resolve.Failure{Key: "K", Ref: ref.Ref{Raw: "keyring://a/b\x1b[2J"}, Err: provider.ErrNotFound}
	got := failureLine(f)
	if strings.ContainsRune(got, 0x1b) {
		t.Fatalf("failureLine = %q holds an escape", got)
	}
	if want := `K  "keyring://a/b\x1b[2J"  not found`; got != want {
		t.Errorf("failureLine = %q, want %q", got, want)
	}
}
