package provider

import "errors"

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
