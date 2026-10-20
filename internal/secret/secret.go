package secret

import (
	"fmt"
	"slices"
)

const redacted = "[redacted]"

// Secret holds a resolved value. Every formatting path prints "[redacted]";
// only Reveal exposes the bytes.
//
// SECURITY: no method other than Reveal may return or print the value.
type Secret struct {
	value []byte
}

// New returns a Secret holding a copy of b.
func New(b []byte) Secret {
	return Secret{value: slices.Clone(b)}
}

// Reveal returns a copy of the value. Call it only where the value is handed
// to the child process.
func (s Secret) Reveal() []byte {
	return slices.Clone(s.value)
}

// String returns "[redacted]".
func (Secret) String() string { return redacted }

// GoString returns "[redacted]" so %#v does not print the value.
func (Secret) GoString() string { return redacted }

// Format writes "[redacted]" for every verb.
func (Secret) Format(f fmt.State, _ rune) {
	_, _ = f.Write([]byte(redacted))
}

// MarshalText returns "[redacted]" so encoders and loggers do not print the
// value.
func (Secret) MarshalText() ([]byte, error) {
	return []byte(redacted), nil
}
