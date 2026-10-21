package provider

import (
	"context"

	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

// Provider resolves references of one scheme.
//
// Errors from Provider and Writer methods wrap one of the sentinel kinds and
// read "<reason>: <kind>". They do not repeat the reference: callers print the
// key and the reference next to them.
type Provider interface {
	Resolve(ctx context.Context, r ref.Ref) (secret.Secret, error)
}

// Writer is implemented by providers that can store and remove values.
type Writer interface {
	Set(ctx context.Context, r ref.Ref, s secret.Secret) error
	Delete(ctx context.Context, r ref.Ref) error
}
