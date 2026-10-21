package providertest

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
)

// Setup connects the suite to the backend behind a provider.
type Setup struct {
	// Put stores value under name in the backend and returns a reference to it.
	Put func(t *testing.T, name, value string) ref.Ref
	// Missing is a reference to a secret that does not exist.
	Missing ref.Ref
}

// Run executes the provider contract as subtests of t.
//
// SECURITY: failure messages never contain a secret value; mismatches report
// "value mismatch" only.
func Run(t *testing.T, p provider.Provider, setup Setup) {
	t.Helper()

	t.Run("resolves a stored value", func(t *testing.T) {
		assertRoundTrip(t, p, setup, "contract-plain", "s3cr3t-value")
	})

	t.Run("round-trips newline and unicode exactly", func(t *testing.T) {
		assertRoundTrip(t, p, setup, "contract-multiline", "line one\nline two\r\nüñî \U0001F99C 日本\n")
	})

	t.Run("missing secret is not found", func(t *testing.T) {
		_, err := p.Resolve(context.Background(), setup.Missing)
		if !errors.Is(err, provider.ErrNotFound) {
			t.Fatalf("Resolve(missing) kind = %q, want %q", provider.Kind(err), provider.ErrNotFound)
		}
		assertOmitsRef(t, err, setup.Missing)
	})

	// Limitation: this subtest resolves Setup.Missing rather than a stored
	// value, because some providers (the in-memory fake) ignore ctx on a hit.
	// It proves the call returns an error and does not hang, not that every
	// provider aborts an in-flight lookup.
	t.Run("canceled context returns an error without hanging", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		done := make(chan error, 1)
		go func() {
			_, err := p.Resolve(ctx, setup.Missing)
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("Resolve with canceled context returned nil error")
			}
			assertOmitsRef(t, err, setup.Missing)
		case <-time.After(10 * time.Second):
			t.Fatal("Resolve with canceled context did not return")
		}
	})
}

// assertOmitsRef fails when err's text contains the reference. Callers print
// the key and the reference next to every provider error, so repeating it
// there only adds noise.
func assertOmitsRef(t *testing.T, err error, r ref.Ref) {
	t.Helper()
	if strings.Contains(err.Error(), r.Raw) {
		t.Fatalf("error text %q repeats the reference %s", err.Error(), r.Raw)
	}
}

func assertRoundTrip(t *testing.T, p provider.Provider, setup Setup, name, value string) {
	t.Helper()
	r := setup.Put(t, name, value)
	got, err := p.Resolve(context.Background(), r)
	if err != nil {
		t.Fatalf("Resolve(%s) failed: kind=%s", r.Raw, provider.Kind(err))
	}
	// SECURITY: test-only comparison against the value the test just stored;
	// a mismatch reports no value.
	if !bytes.Equal(got.Reveal(), []byte(value)) {
		t.Fatalf("Resolve(%s): value mismatch", r.Raw)
	}
}
