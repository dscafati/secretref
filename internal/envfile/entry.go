package envfile

import (
	"fmt"

	"github.com/dscafati/secretref/internal/ref"
)

// Quote records how a value was quoted in the file.
type Quote int

// Quote styles.
const (
	QuoteNone Quote = iota
	QuoteSingle
	QuoteDouble
)

// Entry is one KEY=VALUE line of an env file.
type Entry struct {
	File  string
	Line  int
	Key   string
	Value string // unquoted, escapes processed, NOT expanded
	Quote Quote
	Ref   *ref.Ref // nil if not annotated
}

// ParseError reports a malformed line.
type ParseError struct {
	File string
	Line int
	Msg  string
}

// Error implements error.
func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
}

// Warning reports a line that Parse skipped because it is not a valid
// KEY=VALUE line.
//
// SECURITY: a Warning never holds text from the line; a secret may have been
// pasted there.
type Warning struct {
	File string
	Line int
	Msg  string
}

// String renders the warning as "<file>:<line>: <msg>".
func (w Warning) String() string {
	return fmt.Sprintf("%s:%d: %s", w.File, w.Line, w.Msg)
}
