//go:build acceptance

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/fake"
)

// installTestProviders replaces every provider with one fake loaded from the
// JSON object in SECRETREF_TEST_FAKE_FILE, mapping raw references to values.
// A reference missing from the file resolves to provider.ErrNotFound.
//
// SECURITY: this file is compiled only with the acceptance build tag; release
// binaries never contain it.
func installTestProviders(reg provider.Registry, writers map[string]provider.Writer) error {
	path := os.Getenv("SECRETREF_TEST_FAKE_FILE")
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: test-only file named by the test harness.
	if err != nil {
		return fmt.Errorf("fake provider: %w", err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("fake provider: %s is not a JSON object of strings", path)
	}
	f := fake.New()
	for raw, v := range values {
		f.Put(raw, v)
	}
	for _, scheme := range []string{"aws-sm", "gcp-sm", "keyring"} {
		reg[scheme] = f
	}
	writers["keyring"] = f
	return nil
}
