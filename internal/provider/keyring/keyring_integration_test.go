//go:build integration

package keyring_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/keyring"
	"github.com/dscafati/secretref/internal/provider/providertest"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

// TestOSKeyring runs the contract against the operating system keyring. On
// Linux, `make test-integration` provides a throwaway Secret Service when
// gnome-keyring is installed, so the developer's own keyring is not touched.
func TestOSKeyring(t *testing.T) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	service := "secretref-test-" + hex.EncodeToString(buf)
	p := keyring.New()
	ctx := context.Background()

	requireKeyring(t, p, mustRef(t, "keyring://"+service+"/probe"))

	providertest.Run(t, p, providertest.Setup{
		Put: func(t *testing.T, name, value string) ref.Ref {
			t.Helper()
			r := mustRef(t, "keyring://"+service+"/"+name)
			if err := p.Set(ctx, r, secret.New([]byte(value))); err != nil {
				t.Fatalf("Set failed: kind=%s", provider.Kind(err))
			}
			t.Cleanup(func() { _ = p.Delete(ctx, r) })
			return r
		},
		Missing: mustRef(t, "keyring://"+service+"/missing"),
	})
}

// requireKeyring skips the test when no keyring backend answers, for example
// Linux without a Secret Service on the session bus. A lookup of an entry that
// does not exist must report not-found; anything else means the backend is
// unreachable. With SECRETREF_REQUIRE_KEYRING set, as in CI, it fails instead.
func requireKeyring(t *testing.T, p *keyring.Provider, absent ref.Ref) {
	t.Helper()
	_, err := p.Resolve(context.Background(), absent)
	if errors.Is(err, provider.ErrNotFound) {
		return
	}
	if os.Getenv("SECRETREF_REQUIRE_KEYRING") != "" {
		t.Fatalf("OS keyring unreachable (kind=%s) but SECRETREF_REQUIRE_KEYRING is set", provider.Kind(err))
	}
	t.Skipf("OS keyring unreachable (kind=%s); on Linux install gnome-keyring and run `make test-integration`", provider.Kind(err))
}
