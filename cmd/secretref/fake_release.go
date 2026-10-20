//go:build !acceptance

package main

import "github.com/dscafati/secretref/internal/provider"

// installTestProviders does nothing in release builds. The acceptance build
// replaces it with a version that installs the fake provider.
func installTestProviders(provider.Registry, map[string]provider.Writer) error {
	return nil
}
