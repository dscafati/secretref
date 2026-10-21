package ref_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dscafati/secretref/internal/ref"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		in      string
		want    ref.Ref
		wantErr bool
	}{
		{
			in:   "aws-sm://eu-west-1/prod/db#password",
			want: ref.Ref{Scheme: "aws-sm", Location: "eu-west-1", Name: "prod/db", Field: "password", Raw: "aws-sm://eu-west-1/prod/db#password"},
		},
		{
			in:   "aws-sm://eu-west-1/x?profile=dev",
			want: ref.Ref{Scheme: "aws-sm", Location: "eu-west-1", Name: "x", Opts: map[string]string{"profile": "dev"}, Raw: "aws-sm://eu-west-1/x?profile=dev"},
		},
		{
			in:   "gcp-sm://p/s?version=3",
			want: ref.Ref{Scheme: "gcp-sm", Location: "p", Name: "s", Opts: map[string]string{"version": "3"}, Raw: "gcp-sm://p/s?version=3"},
		},
		{
			in:   "gcp-sm://p/s?version=latest#f",
			want: ref.Ref{Scheme: "gcp-sm", Location: "p", Name: "s", Field: "f", Opts: map[string]string{"version": "latest"}, Raw: "gcp-sm://p/s?version=latest#f"},
		},
		{
			in:   "keyring://myapp/DB_PASSWORD",
			want: ref.Ref{Scheme: "keyring", Location: "myapp", Name: "DB_PASSWORD", Raw: "keyring://myapp/DB_PASSWORD"},
		},
		{
			in:   "aws-sm://r/x#a?b",
			want: ref.Ref{Scheme: "aws-sm", Location: "r", Name: "x", Field: "a?b", Raw: "aws-sm://r/x#a?b"},
		},
		{
			in:   "aws-sm://r/a.b/..c/.d",
			want: ref.Ref{Scheme: "aws-sm", Location: "r", Name: "a.b/..c/.d", Raw: "aws-sm://r/a.b/..c/.d"},
		},
		{
			in:   "keyring://café/ü...",
			want: ref.Ref{Scheme: "keyring", Location: "café", Name: "ü...", Raw: "keyring://café/ü..."},
		},
		// Dot segments.
		{in: "aws-sm://r/../x", wantErr: true},
		{in: "aws-sm://r/a/./x", wantErr: true},
		{in: "aws-sm://r/a/..", wantErr: true},
		{in: "aws-sm://../x", wantErr: true},
		{in: "gcp-sm://p/..", wantErr: true},
		{in: "gcp-sm://./s", wantErr: true},
		{in: "keyring://svc/.", wantErr: true},
		{in: "keyring://../x", wantErr: true},
		// Control and non-printable characters.
		{in: "keyring://a/b\x1b[31m", wantErr: true},
		{in: "keyring://a\x00/b", wantErr: true},
		{in: "keyring://a/b\tc", wantErr: true},
		{in: "keyring://a/b\x7f", wantErr: true},
		{in: "keyring://a/\xff", wantErr: true},
		{in: "keyring://a/b\u0085", wantErr: true},
		{in: "keyring://a/b\u202e", wantErr: true},
		{in: "aws-sm://r/x?profile=a\nb", wantErr: true},
		{in: "aws-sm://r/x#f\rx", wantErr: true},
		{in: "aws-sm://r/x?profile=a%0Ab", wantErr: true},
		{in: "aws-sm://r/x?profile=a%1B%5B2J", wantErr: true},
		{in: "\x1b]0;x\x07keyring://a/b", wantErr: true},
		{in: "aws-sm://r/x/", wantErr: true},
		{in: "aws-sm://r//x", wantErr: true},
		{in: "aws-sm://r/x//y", wantErr: true},
		{in: "gcp-sm://p/a/b", wantErr: true},
		{in: "keyring://svc/a/b", wantErr: true},
		{in: "gcp-sm://p/s?version=%2B3", wantErr: true},
		{in: "gcp-sm://p/s?version=03", wantErr: true},
		{in: "gcp-sm://p/s?version=1.5", wantErr: true},
		{in: "vault://a/b", wantErr: true},
		{in: "no-scheme", wantErr: true},
		{in: "aws-sm://eu-west-1", wantErr: true},
		{in: "aws-sm://eu-west-1/", wantErr: true},
		{in: "aws-sm:///name", wantErr: true},
		{in: "keyring://svc", wantErr: true},
		{in: "gcp-sm://p/s?version=x", wantErr: true},
		{in: "gcp-sm://p/s?version=0", wantErr: true},
		{in: "gcp-sm://p/s?version=-1", wantErr: true},
		{in: "aws-sm://r/x?bogus=1", wantErr: true},
		{in: "aws-sm://r/x?version=1", wantErr: true},
		{in: "keyring://a/b?profile=x", wantErr: true},
		{in: "aws-sm://r/x#", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ref.Parse(tt.in)
			if tt.wantErr {
				var perr *ref.Error
				if !errors.As(err, &perr) {
					t.Fatalf("Parse(%q) error = %v, want *ref.Error", tt.in, err)
				}
				if perr.Raw != tt.in {
					t.Errorf("Error.Raw = %q, want %q", perr.Raw, tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Parse(%q) mismatch (-want +got):\n%s", tt.in, diff)
			}
			if got.String() != tt.in {
				t.Errorf("String() = %q, want %q", got.String(), tt.in)
			}
		})
	}
}

func TestErrorMessage(t *testing.T) {
	e := &ref.Error{Raw: "vault://a/b", Msg: "unknown scheme"}
	if got, want := e.Error(), "invalid reference vault://a/b: unknown scheme"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestOutputCannotCarryEscapes(t *testing.T) {
	const raw = "keyring://a/b\x1b[2J\x07\xff"
	e := &ref.Error{Raw: raw, Msg: "bad"}
	if got, want := e.Error(), `invalid reference "keyring://a/b\x1b[2J\a\xff": bad`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got, want := (ref.Ref{Raw: raw}).String(), `"keyring://a/b\x1b[2J\a\xff"`; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got := (ref.Ref{Raw: "keyring://café/b"}).String(); got != "keyring://café/b" {
		t.Errorf("printable String() = %q", got)
	}
}

func TestSchemes(t *testing.T) {
	got := ref.Schemes()
	if diff := cmp.Diff([]string{"aws-sm", "gcp-sm", "keyring"}, got); diff != "" {
		t.Errorf("Schemes() (-want +got):\n%s", diff)
	}
	got[0] = "changed"
	if ref.Schemes()[0] != "aws-sm" {
		t.Error("Schemes() returned a shared slice")
	}
	for _, s := range ref.Schemes() {
		if _, err := ref.Parse(s + "://loc/name"); err != nil {
			t.Errorf("Parse with scheme %s: %v", s, err)
		}
	}
}
