package keyring

import (
	"context"
	"errors"
	"fmt"

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
type Provider struct{}

// New returns a keyring Provider.
func New() *Provider { return &Provider{} }

// Resolve returns the stored value for r.
func (*Provider) Resolve(ctx context.Context, r ref.Ref) (secret.Secret, error) {
	if err := check(ctx, r); err != nil {
		return secret.Secret{}, err
	}
	v, err := gokeyring.Get(r.Location, r.Name)
	if err != nil {
		return secret.Secret{}, mapErr(err)
	}
	return secret.New([]byte(v)), nil
}

// Set stores s under r.
func (*Provider) Set(ctx context.Context, r ref.Ref, s secret.Secret) error {
	if err := check(ctx, r); err != nil {
		return err
	}
	if err := gokeyring.Set(r.Location, r.Name, string(s.Reveal())); err != nil {
		return mapErr(err)
	}
	return nil
}

// Delete removes the entry for r.
func (*Provider) Delete(ctx context.Context, r ref.Ref) error {
	if err := check(ctx, r); err != nil {
		return err
	}
	if err := gokeyring.Delete(r.Location, r.Name); err != nil {
		return mapErr(err)
	}
	return nil
}

func check(ctx context.Context, r ref.Ref) error {
	if r.Scheme != scheme {
		return fmt.Errorf("keyring provider cannot handle scheme %q: %w", r.Scheme, provider.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", err, provider.ErrUnavailable)
	}
	return nil
}

// mapErr drops the backend's error text: it is not needed to act on the
// failure and could echo backend internals.
func mapErr(err error) error {
	if errors.Is(err, gokeyring.ErrNotFound) {
		return provider.ErrNotFound
	}
	return fmt.Errorf("OS keyring failed: %w", provider.ErrUnavailable)
}
