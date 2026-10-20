package provider

import "fmt"

// Registry maps a reference scheme to its Provider.
type Registry map[string]Provider

// Lookup returns the provider for scheme. An unknown scheme wraps ErrInvalid.
func (g Registry) Lookup(scheme string) (Provider, error) {
	p, ok := g[scheme]
	if !ok {
		return nil, fmt.Errorf("no provider for scheme %q: %w", scheme, ErrInvalid)
	}
	return p, nil
}
