package keyring_test

import (
	"context"
	"errors"
	"testing"

	gokeyring "github.com/zalando/go-keyring"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/keyring"
	"github.com/dscafati/secretref/internal/provider/providertest"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

func mustRef(t *testing.T, raw string) ref.Ref {
	t.Helper()
	r, err := ref.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestContract(t *testing.T) {
	gokeyring.MockInit()
	p := keyring.New()
	providertest.Run(t, p, providertest.Setup{
		Put: func(t *testing.T, name, value string) ref.Ref {
			t.Helper()
			r := mustRef(t, "keyring://secretref-unit/"+name)
			if err := p.Set(context.Background(), r, secret.New([]byte(value))); err != nil {
				t.Fatalf("Set failed: kind=%s", provider.Kind(err))
			}
			return r
		},
		Missing: mustRef(t, "keyring://secretref-unit/missing"),
	})
}

func TestSetResolveDelete(t *testing.T) {
	gokeyring.MockInit()
	p := keyring.New()
	ctx := context.Background()
	r := mustRef(t, "keyring://svc/acct")

	if err := p.Set(ctx, r, secret.New([]byte("v1"))); err != nil {
		t.Fatal(err)
	}
	got, err := p.Resolve(ctx, r)
	if err != nil || string(got.Reveal()) != "v1" {
		t.Fatalf("Resolve after Set: err=%v", err)
	}
	if err := p.Delete(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Resolve(ctx, r); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Resolve after Delete kind = %q, want not found", provider.Kind(err))
	}
	if err := p.Delete(ctx, r); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Delete of missing kind = %q, want not found", provider.Kind(err))
	}
}

func TestNonKeyringSchemeIsInvalid(t *testing.T) {
	gokeyring.MockInit()
	p := keyring.New()
	r := mustRef(t, "aws-sm://us-east-1/db")
	if err := p.Set(context.Background(), r, secret.New([]byte("x"))); !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("Set kind = %q, want invalid", provider.Kind(err))
	}
	if err := p.Delete(context.Background(), r); !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("Delete kind = %q, want invalid", provider.Kind(err))
	}
	if _, err := p.Resolve(context.Background(), r); !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("Resolve kind = %q, want invalid", provider.Kind(err))
	}
}

func TestBackendFailureIsUnavailable(t *testing.T) {
	gokeyring.MockInitWithError(errors.New("backend exploded"))
	t.Cleanup(gokeyring.MockInit)
	p := keyring.New()
	_, err := p.Resolve(context.Background(), mustRef(t, "keyring://svc/acct"))
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Resolve kind = %q, want unavailable", provider.Kind(err))
	}
	if want := "OS keyring failed: unavailable"; err.Error() != want {
		t.Fatalf("Resolve error = %q, want %q", err.Error(), want)
	}
}

func TestNotFoundText(t *testing.T) {
	gokeyring.MockInit()
	_, err := keyring.New().Resolve(context.Background(), mustRef(t, "keyring://svc/acct"))
	if err == nil || err.Error() != "not found" {
		t.Fatalf("Resolve error = %v, want \"not found\"", err)
	}
}
