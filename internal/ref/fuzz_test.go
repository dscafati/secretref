package ref_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"

	"github.com/dscafati/secretref/internal/ref"
)

func isPrintable(s string) bool {
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

func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"aws-sm://eu-west-1/prod/db?profile=dev#password",
		"gcp-sm://p/s?version=latest#f",
		"keyring://myapp/DB_PASSWORD",
		"keyring://a/b\x1b[31m",
		"gcp-sm://p/..",
		"aws-sm://r/a/./b",
		"aws-sm://r/x?profile=a%0Ab",
		"keyring://café/ü",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r, err := ref.Parse(s)
		if err != nil {
			var perr *ref.Error
			if !errors.As(err, &perr) {
				t.Fatalf("error %T is not *ref.Error", err)
			}
			if perr.Raw != s {
				t.Fatalf("Error.Raw = %q, want %q", perr.Raw, s)
			}
			// SECURITY: an error is printed on the terminal and must not
			// carry control characters from the input.
			if !isPrintable(err.Error()) {
				t.Fatalf("error text is not printable: %q", err.Error())
			}
			return
		}
		if !isPrintable(s) {
			t.Fatalf("accepted a non-printable reference %q", s)
		}
		if r.String() != s || r.Raw != s {
			t.Fatalf("String() = %q, Raw = %q, want %q", r.String(), r.Raw, s)
		}
		if !slices.Contains(ref.Schemes(), r.Scheme) {
			t.Fatalf("unknown scheme %q accepted", r.Scheme)
		}
		for _, seg := range append([]string{r.Location}, strings.Split(r.Name, "/")...) {
			if seg == "" || seg == "." || seg == ".." {
				t.Fatalf("segment %q accepted in %q", seg, s)
			}
		}
		if r.Scheme != "aws-sm" && strings.Contains(r.Name, "/") {
			t.Fatalf("%s name with / accepted: %q", r.Scheme, s)
		}
		for k, v := range r.Opts {
			if !isPrintable(k) || !isPrintable(v) {
				t.Fatalf("non-printable option %q=%q", k, v)
			}
		}
		again, err := ref.Parse(r.Raw)
		if err != nil {
			t.Fatalf("Parse(Raw) failed: %v", err)
		}
		if diff := cmp.Diff(r, again); diff != "" {
			t.Fatalf("Parse(Raw) differs (-first +again):\n%s", diff)
		}
	})
}
