package provider

import (
	"context"

	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

// Provider resolves references of one scheme.
type Provider interface {
	Resolve(ctx context.Context, r ref.Ref) (secret.Secret, error)
}

// Writer is implemented by providers that can store and remove values.
type Writer interface {
	Set(ctx context.Context, r ref.Ref, s secret.Secret) error
	Delete(ctx context.Context, r ref.Ref) error
}
