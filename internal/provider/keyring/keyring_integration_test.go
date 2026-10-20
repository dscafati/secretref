//go:build integration

package keyring_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/keyring"
	"github.com/dscafati/secretref/internal/provider/providertest"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

// TestOSKeyring runs the contract against the real operating system keyring.
func TestOSKeyring(t *testing.T) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	service := "secretref-test-" + hex.EncodeToString(buf)
	p := keyring.New()
	ctx := context.Background()

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
