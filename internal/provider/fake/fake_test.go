package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/fake"
	"github.com/dscafati/secretref/internal/provider/providertest"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

func TestFakeContract(t *testing.T) {
	f := fake.New()
	providertest.Run(t, f, providertest.Setup{
		Put: func(t *testing.T, name, value string) ref.Ref {
			t.Helper()
			r := ref.Ref{Scheme: "keyring", Location: "svc", Name: name, Raw: "keyring://svc/" + name}
			f.Put(r.Raw, value)
			return r
		},
		Missing: ref.Ref{Scheme: "keyring", Location: "svc", Name: "absent", Raw: "keyring://svc/absent"},
	})
}

func TestFakeFailAndWriter(t *testing.T) {
	f := fake.New()
	r := ref.Ref{Raw: "keyring://svc/a"}
	f.Fail(r.Raw, provider.ErrDenied)
	if _, err := f.Resolve(context.Background(), r); !errors.Is(err, provider.ErrDenied) {
		t.Fatalf("Resolve kind = %q, want denied", provider.Kind(err))
	}
	if err := f.Set(context.Background(), r, secret.New([]byte("v"))); err != nil {
		t.Fatal(err)
	}
	if err := f.Delete(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := f.Delete(context.Background(), r); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("second Delete kind = %q, want not found", provider.Kind(err))
	}
}
