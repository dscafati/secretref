package provider_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

func TestKind(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{provider.ErrNotFound, "not found"},
		{provider.ErrDenied, "denied"},
		{provider.ErrUnavailable, "unavailable"},
		{provider.ErrInvalid, "invalid"},
		{fmt.Errorf("aws-sm://r/x: %w", provider.ErrNotFound), "not found"},
		{fmt.Errorf("wrap: %w", fmt.Errorf("inner: %w", provider.ErrDenied)), "denied"},
		{errors.New("boom"), "error"},
		{nil, "error"},
	}
	for _, tt := range tests {
		if got := provider.Kind(tt.err); got != tt.want {
			t.Errorf("Kind(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestInterrupted(t *testing.T) {
	if err := provider.Interrupted(context.Background()); err != nil {
		t.Fatalf("Interrupted(live ctx) = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := provider.Interrupted(ctx)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Interrupted(canceled) = %v, want context.Canceled and unavailable", err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()
	if err := provider.Interrupted(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Interrupted(expired) = %v, want context.DeadlineExceeded", err)
	}
}

type stubProvider struct{}

func (stubProvider) Resolve(context.Context, ref.Ref) (secret.Secret, error) {
	return secret.New(nil), nil
}

func TestRegistryLookupUnknown(t *testing.T) {
	reg := provider.Registry{"keyring": stubProvider{}}

	if _, err := reg.Lookup("keyring"); err != nil {
		t.Fatalf("Lookup(keyring): %v", err)
	}
	_, err := reg.Lookup("vault")
	if !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("Lookup(vault) error = %v, want it to wrap ErrInvalid", err)
	}
	// The caller prints the reference, which names the scheme.
	if want := "no provider for this scheme: invalid"; err.Error() != want {
		t.Fatalf("Lookup(vault) error = %q, want %q", err.Error(), want)
	}
}
