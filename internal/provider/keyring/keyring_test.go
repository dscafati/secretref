package keyring_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

// blockingBackend never answers until release is closed, like a Secret
// Service waiting on an unlock prompt nobody sees.
type blockingBackend struct{ release chan struct{} }

func (b blockingBackend) Get(string, string) (string, error) { <-b.release; return canary, nil }
func (b blockingBackend) Set(string, string, string) error   { <-b.release; return nil }
func (b blockingBackend) Delete(string, string) error        { <-b.release; return nil }

const canary = "SR_CANARY_7f3a9c"

func TestCallsReturnWhenContextEnds(t *testing.T) {
	b := blockingBackend{release: make(chan struct{})}
	t.Cleanup(func() { close(b.release) })
	p := keyring.NewWithBackend(b)
	r := mustRef(t, "keyring://svc/acct")
	calls := []struct {
		name string
		call func(context.Context) error
	}{
		{"Resolve", func(ctx context.Context) error { _, err := p.Resolve(ctx, r); return err }},
		{"Set", func(ctx context.Context) error { return p.Set(ctx, r, secret.New([]byte(canary))) }},
		{"Delete", func(ctx context.Context) error { return p.Delete(ctx, r) }},
	}
	for _, tt := range calls {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- tt.call(ctx) }()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, provider.ErrUnavailable) {
					t.Fatalf("%s error = %v, want context.DeadlineExceeded and unavailable", tt.name, err)
				}
				if strings.Contains(err.Error(), canary) {
					t.Fatalf("%s error leaks the value", tt.name)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s did not return after the context ended", tt.name)
			}
		})
	}
}

// memBackend is an in-memory Backend.
type memBackend map[string]string

func (m memBackend) Get(service, user string) (string, error) {
	v, ok := m[service+"/"+user]
	if !ok {
		return "", provider.ErrNotFound
	}
	return v, nil
}

func (m memBackend) Set(service, user, password string) error {
	m[service+"/"+user] = password
	return nil
}

func (m memBackend) Delete(service, user string) error {
	if _, ok := m[service+"/"+user]; !ok {
		return provider.ErrNotFound
	}
	delete(m, service+"/"+user)
	return nil
}

func TestBackendIsInjected(t *testing.T) {
	m := memBackend{"svc/acct": "v1"}
	p := keyring.NewWithBackend(m)
	ctx := context.Background()
	got, err := p.Resolve(ctx, mustRef(t, "keyring://svc/acct"))
	if err != nil || string(got.Reveal()) != "v1" {
		t.Fatalf("Resolve: err=%v", err)
	}
	if err := p.Set(ctx, mustRef(t, "keyring://svc/new"), secret.New([]byte("v2"))); err != nil || m["svc/new"] != "v2" {
		t.Fatalf("Set: err=%v", err)
	}
	if err := p.Delete(ctx, mustRef(t, "keyring://svc/acct")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := p.Resolve(ctx, mustRef(t, "keyring://svc/acct")); err == nil || err.Error() != "not found" {
		t.Fatalf("Resolve after Delete error = %v, want \"not found\"", err)
	}
}

func TestNotFoundText(t *testing.T) {
	gokeyring.MockInit()
	_, err := keyring.New().Resolve(context.Background(), mustRef(t, "keyring://svc/acct"))
	if err == nil || err.Error() != "not found" {
		t.Fatalf("Resolve error = %v, want \"not found\"", err)
	}
}

type bigBackend struct{ keyring.Backend }

func (bigBackend) Set(string, string, string) error { return gokeyring.ErrSetDataTooBig }

func TestSetTooBigIsInvalidWithHint(t *testing.T) {
	p := keyring.NewWithBackend(bigBackend{})
	err := p.Set(context.Background(), mustRef(t, "keyring://svc/acct"), secret.New([]byte("v")))
	if !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("kind = %q, want invalid", provider.Kind(err))
	}
	if !strings.Contains(err.Error(), "aws-sm or gcp-sm") {
		t.Errorf("error lacks the hint: %v", err)
	}
}
