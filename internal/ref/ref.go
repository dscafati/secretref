package ref

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Ref is a parsed secret reference.
type Ref struct {
	Scheme   string            // one of Schemes()
	Location string            // region | project | service
	Name     string            // secret name (may contain "/") | secret | account
	Field    string            // JSON field after "#", "" if none
	Opts     map[string]string // query params: profile, version
	Raw      string
}

// Error reports a reference that does not parse.
type Error struct {
	Raw string
	Msg string
}

// Error implements error. Raw is shown quoted and escaped when it holds a
// character that is not printable.
func (e *Error) Error() string {
	return fmt.Sprintf("invalid reference %s: %s", display(e.Raw), e.Msg)
}

// schemeOpts maps every supported scheme to the query keys a reference with
// that scheme may carry. It is the single list of schemes; Schemes and
// allowedOpts derive from it.
func schemeOpts() map[string][]string {
	return map[string][]string{
		"aws-sm":  {"profile"},
		"gcp-sm":  {"version"},
		"keyring": nil,
	}
}

// Schemes returns the supported reference schemes in sorted order. The slice
// is new on every call, so the caller may modify it.
func Schemes() []string {
	return slices.Sorted(maps.Keys(schemeOpts()))
}

// allowedOpts returns the query keys a reference with scheme may carry, and
// false when the scheme is unknown.
func allowedOpts(scheme string) (map[string]bool, bool) {
	keys, ok := schemeOpts()[scheme]
	if !ok {
		return nil, false
	}
	allowed := make(map[string]bool, len(keys))
	for _, k := range keys {
		allowed[k] = true
	}
	return allowed, true
}

// Parse validates s and returns the reference it names. Errors have type
// *Error.
func Parse(s string) (Ref, error) {
	fail := func(format string, args ...any) (Ref, error) {
		return Ref{}, &Error{Raw: s, Msg: fmt.Sprintf(format, args...)}
	}

	// SECURITY: references are printed by check, run, set, and delete. A
	// control character could drive the terminal, and invisible or
	// direction-changing characters could disguise the reference.
	if !printable(s) {
		return fail("contains a control or non-printable character")
	}
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok {
		return fail("missing scheme")
	}
	allowed, known := allowedOpts(scheme)
	if !known {
		return fail("unknown scheme %q", scheme)
	}

	rest, field, hasField := strings.Cut(rest, "#")
	if hasField && field == "" {
		return fail("empty field after #")
	}
	rest, query, hasQuery := strings.Cut(rest, "?")

	location, name, ok := strings.Cut(rest, "/")
	if !ok || location == "" || name == "" {
		return fail("want %s://<location>/<name>", scheme)
	}

	if scheme != "aws-sm" && strings.Contains(name, "/") {
		return fail("%s name must not contain \"/\"", scheme)
	}
	segments := append([]string{location}, strings.Split(name, "/")...)
	if slices.Contains(segments, "") {
		return fail("name must not have an empty path segment")
	}
	// SECURITY: providers build request paths from these segments; a "." or
	// ".." segment would address a different resource.
	if slices.Contains(segments, ".") || slices.Contains(segments, "..") {
		return fail("location and name must not have a \".\" or \"..\" segment")
	}

	var opts map[string]string
	if hasQuery {
		values, err := url.ParseQuery(query)
		if err != nil {
			return fail("malformed query")
		}
		if len(values) == 0 {
			return fail("empty query")
		}
		opts = make(map[string]string, len(values))
		for k, v := range values {
			if !allowed[k] {
				return fail("unknown query key %q for %s", k, scheme)
			}
			if len(v) != 1 || v[0] == "" {
				return fail("query key %q needs exactly one non-empty value", k)
			}
			if !printable(v[0]) {
				return fail("query key %q has a control or non-printable character", k)
			}
			opts[k] = v[0]
		}
	}
	if v, ok := opts["version"]; ok && v != "latest" {
		if !isPositiveInt(v) {
			return fail("version %q must be a positive integer without sign or leading zeros, or \"latest\"", v)
		}
	}

	return Ref{
		Scheme:   scheme,
		Location: location,
		Name:     name,
		Field:    field,
		Opts:     opts,
		Raw:      s,
	}, nil
}

// String returns the reference as it was written. Parse accepts only
// printable text; for a Ref built some other way, text that is not printable
// is shown quoted and escaped, so the result cannot carry terminal escapes.
func (r Ref) String() string { return display(r.Raw) }

// printable reports whether s is valid UTF-8 made only of printable
// characters (unicode.IsPrint), which excludes control, format, and invisible
// characters.
func printable(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if !unicode.IsPrint(c) {
			return false
		}
	}
	return true
}

// display returns s unchanged when it is printable, and as a Go quoted string
// otherwise.
func display(s string) string {
	if printable(s) {
		return s
	}
	return strconv.Quote(s)
}

// isPositiveInt reports whether v is digits only, starts with 1-9.
func isPositiveInt(v string) bool {
	if v == "" || v[0] == '0' {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
