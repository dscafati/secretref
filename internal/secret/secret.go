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
//
// SECURITY: the bytes sit behind a pointer. fmt cannot call Format on a Secret
// stored in an unexported struct field, so it prints the field reflectively;
// with a pointer that shows an address, not the bytes.
type Secret struct {
	v *value
}

type value struct {
	b []byte
}

// New returns a Secret holding a copy of b.
func New(b []byte) Secret {
	return Secret{v: &value{b: slices.Clone(b)}}
}

// Reveal returns a copy of the value. Call it only where the value is handed
// to the child process.
func (s Secret) Reveal() []byte {
	if s.v == nil {
		return []byte{}
	}
	return slices.Clone(s.v.b)
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
