package fake

import (
	"context"
	"slices"
	"sync"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

var (
	_ provider.Provider = (*Fake)(nil)
	_ provider.Writer   = (*Fake)(nil)
)

type result struct {
	value secret.Secret
	err   error
}

// Fake is a provider.Provider and provider.Writer keyed by the raw reference
// string. It is safe for concurrent use.
type Fake struct {
	mu      sync.Mutex
	results map[string]result
	calls   []string
}

// New returns an empty Fake.
func New() *Fake {
	return &Fake{results: map[string]result{}}
}

// Put makes raw resolve to value.
func (f *Fake) Put(raw string, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[raw] = result{value: secret.New([]byte(value))}
}

// Fail makes raw resolve to err.
func (f *Fake) Fail(raw string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[raw] = result{err: err}
}

// Calls returns the raw references resolved so far, in the order received.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// Resolve returns the configured result for r.Raw. An unknown reference
// returns provider.ErrNotFound.
func (f *Fake) Resolve(_ context.Context, r ref.Ref) (secret.Secret, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Raw)
	res, ok := f.results[r.Raw]
	if !ok {
		return secret.Secret{}, provider.ErrNotFound
	}
	return res.value, res.err
}

// Set stores s under r.Raw.
func (f *Fake) Set(_ context.Context, r ref.Ref, s secret.Secret) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[r.Raw] = result{value: s}
	return nil
}

// Delete removes r.Raw. A missing reference returns provider.ErrNotFound.
func (f *Fake) Delete(_ context.Context, r ref.Ref) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.results[r.Raw]; !ok {
		return provider.ErrNotFound
	}
	delete(f.results, r.Raw)
	return nil
}
