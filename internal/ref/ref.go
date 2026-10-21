package ref

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
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

// Error implements error.
func (e *Error) Error() string {
	return fmt.Sprintf("invalid reference %s: %s", e.Raw, e.Msg)
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
	if scheme == "aws-sm" && slices.Contains(strings.Split(name, "/"), "") {
		return fail("name must not have an empty path segment")
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

// String returns the reference as it was written.
func (r Ref) String() string { return r.Raw }

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
