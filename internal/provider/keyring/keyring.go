package keyring

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	gokeyring "github.com/zalando/go-keyring"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

var (
	_ provider.Provider = (*Provider)(nil)
	_ provider.Writer   = (*Provider)(nil)
)

const scheme = "keyring"

// Provider reads and writes the OS keyring. A reference keyring://<service>/<account>
// maps to the keyring entry with that service and user.
type Provider struct {
	backend Backend
}

// New returns a keyring Provider for the OS keyring.
func New() *Provider { return NewWithBackend(osBackend{}) }

// NewWithBackend returns a keyring Provider that stores entries in b.
func NewWithBackend(b Backend) *Provider { return &Provider{backend: b} }

// Resolve returns the stored value for r. It returns once ctx is done even if
// the keyring has not answered; see call.
func (p *Provider) Resolve(ctx context.Context, r ref.Ref) (secret.Secret, error) {
	if err := check(ctx, r); err != nil {
		return secret.Secret{}, err
	}
	v, err := call(ctx, func() (string, error) { return p.backend.Get(r.Location, r.Name) })
	if err != nil {
		return secret.Secret{}, err
	}
	return secret.New([]byte(v)), nil
}

// Set stores s under r. It returns once ctx is done; see call.
func (p *Provider) Set(ctx context.Context, r ref.Ref, s secret.Secret) error {
	if err := check(ctx, r); err != nil {
		return err
	}
	_, err := call(ctx, func() (struct{}, error) {
		// SECURITY: the OS keyring API takes a string; this is the stored value
		// itself, passed to the backend and never logged or returned.
		return struct{}{}, p.backend.Set(r.Location, r.Name, string(s.Reveal()))
	})
	return err
}

// Delete removes the entry for r. It returns once ctx is done; see call.
func (p *Provider) Delete(ctx context.Context, r ref.Ref) error {
	if err := check(ctx, r); err != nil {
		return err
	}
	_, err := call(ctx, func() (struct{}, error) {
		return struct{}{}, p.backend.Delete(r.Location, r.Name)
	})
	return err
}

func check(ctx context.Context, r ref.Ref) error {
	if r.Scheme != scheme {
		return fmt.Errorf("keyring provider cannot handle scheme %q: %w", r.Scheme, provider.ErrInvalid)
	}
	return provider.Interrupted(ctx)
}

// call runs fn, a backend call, and returns its mapped result, or the
// context's error as soon as ctx is done. OS keyring APIs take no context and
// can block indefinitely (a Secret Service unlock prompt, a locked D-Bus), so
// fn runs in its own goroutine. After an early return fn keeps running in the
// background until the backend answers; its result goes to a buffered channel
// that nobody reads, so the goroutine never leaks past that answer.
func call[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := fn()
		done <- result{v, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			var zero T
			return zero, mapErr(res.err)
		}
		return res.v, nil
	case <-ctx.Done():
		var zero T
		return zero, provider.Interrupted(ctx)
	}
}

// mapErr drops the backend's error text: it is not needed to act on the
// failure and could echo backend internals.
func mapErr(err error) error {
	if errors.Is(err, provider.ErrNotFound) {
		return provider.ErrNotFound
	}
	if errors.Is(err, gokeyring.ErrSetDataTooBig) {
		return fmt.Errorf("value too large for the OS keyring (%s limit ~%d bytes); store it in aws-sm or gcp-sm instead: %w",
			runtime.GOOS, sizeLimit(runtime.GOOS), provider.ErrInvalid)
	}
	return fmt.Errorf("OS keyring failed: %w", provider.ErrUnavailable)
}

// sizeLimit is the approximate largest value go-keyring stores on goos.
// Windows Credential Manager takes 2560 bytes; on macOS the value is
// base64-encoded into a 4096-byte security command.
func sizeLimit(goos string) int {
	if goos == "windows" {
		return 2560
	}
	return 3000
}
