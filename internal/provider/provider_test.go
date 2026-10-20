package provider_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
}
