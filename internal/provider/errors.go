package provider

import (
	"context"
	"errors"
	"fmt"
)

// Sentinel error kinds. Provider errors wrap one of these with %w.
var (
	ErrNotFound    = errors.New("not found")
	ErrDenied      = errors.New("denied")
	ErrUnavailable = errors.New("unavailable")
	ErrInvalid     = errors.New("invalid")
)

// Kind returns the name of the sentinel kind err wraps, or "error" if it wraps
// none.
func Kind(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return ErrNotFound.Error()
	case errors.Is(err, ErrDenied):
		return ErrDenied.Error()
	case errors.Is(err, ErrUnavailable):
		return ErrUnavailable.Error()
	case errors.Is(err, ErrInvalid):
		return ErrInvalid.Error()
	}
	return "error"
}

// Interrupted returns nil while ctx is live. Once ctx is done it returns an
// error that wraps both ctx.Err() and ErrUnavailable, so the resolver can
// report a timeout or cancellation instead of a generic backend failure.
// Providers check it before classifying a transport error.
func Interrupted(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", err, ErrUnavailable)
	}
	return nil
}
