package provider

import "fmt"

// Registry maps a reference scheme to its Provider.
type Registry map[string]Provider

// Lookup returns the provider for scheme. An unknown scheme wraps ErrInvalid.
// The error does not name the scheme: callers print the reference, which does.
func (g Registry) Lookup(scheme string) (Provider, error) {
	p, ok := g[scheme]
	if !ok {
		return nil, fmt.Errorf("no provider for this scheme: %w", ErrInvalid)
	}
	return p, nil
}
