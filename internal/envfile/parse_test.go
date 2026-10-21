package envfile

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

//nolint:gochecknoglobals // Standard golden-file test flag.
var update = flag.Bool("update", false, "rewrite golden files")

// goldenEntry is the stable JSON shape of an Entry in golden files.
type goldenEntry struct {
	Line  int     `json:"line"`
	Key   string  `json:"key"`
	Value string  `json:"value"`
	Quote string  `json:"quote"`
	Ref   *string `json:"ref"`
}

func render(entries []Entry) []byte {
	out := make([]goldenEntry, 0, len(entries))
	for _, e := range entries {
		g := goldenEntry{Line: e.Line, Key: e.Key, Value: e.Value}
		switch e.Quote {
		case QuoteNone:
			g.Quote = "none"
		case QuoteSingle:
			g.Quote = "single"
		case QuoteDouble:
			g.Quote = "double"
		}
		if e.Ref != nil {
			raw := e.Ref.Raw
			g.Ref = &raw
		}
		out = append(out, g)
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		panic(err) //nolint:forbidigo // golden-file generator in a test helper cannot take *testing.T; the panic surfaces a test setup bug
	}
	return append(b, '\n')
}

func TestParseGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/*.env")
	if err != nil || len(files) == 0 {
		t.Fatalf("no testdata: %v", err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, err := os.Open(path) //nolint:gosec // Fixed testdata glob.
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			src, err := io.ReadAll(f)
			if err != nil {
				t.Fatal(err)
			}
			// Git may normalize line endings, so CRLF input is built here.
			lf := strings.ReplaceAll(string(src), "\r\n", "\n")
			entries, warnings, err := Parse(path, strings.NewReader(lf))
			if err != nil || len(warnings) != 0 {
				t.Fatalf("Parse: %v, warnings %v", err, warnings)
			}
			crlf, _, err := Parse(path, strings.NewReader(strings.ReplaceAll(lf, "\n", "\r\n")))
			if err != nil {
				t.Fatalf("Parse CRLF: %v", err)
			}
			if diff := cmp.Diff(string(render(entries)), string(render(crlf))); diff != "" {
				t.Errorf("CRLF differs from LF (-lf +crlf):\n%s", diff)
			}
			got := render(entries)
			golden := strings.TrimSuffix(path, ".env") + ".golden"
			if *update {
				if err := os.WriteFile(golden, got, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden) //nolint:gosec // Fixed testdata glob.
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(string(want), string(got)); diff != "" {
				t.Errorf("golden mismatch (-want +got):\n%s", diff)
			}
			for _, e := range entries {
				if e.File != path {
					t.Errorf("Entry.File = %q, want %q", e.File, path)
				}
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		line int
		msg  string
	}{
		{"annotation then blank", "# secretref: keyring://a/b\n\nK=v\n", 1, "annotation has no variable directly below it"},
		{"annotation then comment", "# secretref: keyring://a/b\n# note\nK=v\n", 1, "annotation has no variable directly below it"},
		{"annotation at EOF", "A=1\n# secretref: keyring://a/b", 2, "annotation has no variable directly below it"},
		{"annotation then annotation", "# secretref: keyring://a/b\n# secretref: keyring://c/d\nK=v\n", 1, "annotation has no variable directly below it"},
		{"invalid ref", "A=1\n# secretref: nope://x/y\nK=v\n", 2, "invalid reference"},
		{"missing ref", "# secretref:\nK=v\n", 1, "annotation needs exactly one reference"},
		{"extra tokens", "# secretref: keyring://a/b extra\nK=v\n", 1, "annotation needs exactly one reference"},
		// A malformed line directly under an annotation stays an error.
		{"annotated invalid key", "# secretref: keyring://a/b\n1ABC=x\n", 2, "invalid variable name"},
		{"annotated missing equals", "# secretref: keyring://a/b\nJUSTKEY\n", 2, "expected '=' after variable name"},
		{"annotated junk after quote", "# secretref: keyring://a/b\nA=\"x\" y\n", 2, "unexpected text after quoted value"},
		{"annotated unterminated double", "# secretref: keyring://a/b\nB=\"abc\nC=2\n", 2, "unterminated double-quoted value"},
		{"annotated unterminated single", "# secretref: keyring://a/b\nB='abc\n", 2, "unterminated single-quoted value"},
		{"annotated empty key", "# secretref: keyring://a/b\n=x\n", 2, "invalid variable name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Parse("f.env", strings.NewReader(tt.in))
			pe, ok := err.(*ParseError) //nolint:errorlint // Parse returns *ParseError directly.
			if !ok {
				t.Fatalf("err = %v (%T), want *ParseError", err, err)
			}
			if pe.File != "f.env" || pe.Line != tt.line {
				t.Errorf("got %s:%d, want f.env:%d", pe.File, pe.Line, tt.line)
			}
			if !strings.Contains(pe.Msg, tt.msg) {
				t.Errorf("Msg = %q, want it to contain %q", pe.Msg, tt.msg)
			}
		})
	}
}

func TestParseSkipsMalformedLines(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		keys  []string // keys of the entries, in order
		lines []int    // lines of the warnings
	}{
		{"invalid key", "1ABC=x\nB=2\n", []string{"B"}, []int{1}},
		{"missing equals", "A=1\nJUSTKEY\nB=2\n", []string{"A", "B"}, []int{2}},
		{"export without equals", "export KEY\nB=2\n", []string{"B"}, []int{1}},
		{"junk after quote", "A=\"x\" y\nB=2\n", []string{"B"}, []int{1}},
		{"empty key", "=x\n", nil, []int{1}},
		{"space in key", "MY KEY=x\n", nil, []int{1}},
		{"backtick", "A=`x`y`\nB=2\n", []string{"A", "B"}, nil},
		// An unterminated quote skips only its own line; the lines after it
		// parse on their own.
		{"unterminated double", "A=1\nB=\"abc\nC=2\n", []string{"A", "C"}, []int{2}},
		{"unterminated single", "A='abc\nB=2\n", []string{"B"}, []int{1}},
		{"annotation after skipped line", "1X=y\n# secretref: keyring://a/b\nK=v\n", []string{"K"}, []int{1}},
		{"several", "1X=y\nA=1\n2Y=z\n", []string{"A"}, []int{1, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, warnings, err := Parse("f.env", strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			var keys []string
			for _, e := range entries {
				keys = append(keys, e.Key)
			}
			if diff := cmp.Diff(tt.keys, keys); diff != "" {
				t.Errorf("keys (-want +got):\n%s", diff)
			}
			var lines []int
			for _, w := range warnings {
				lines = append(lines, w.Line)
				if w.File != "f.env" {
					t.Errorf("warning file %q", w.File)
				}
				if got, want := w.String(), fmt.Sprintf("f.env:%d: skipped unparseable line", w.Line); got != want {
					t.Errorf("warning = %q, want %q", got, want)
				}
			}
			if diff := cmp.Diff(tt.lines, lines); diff != "" {
				t.Errorf("warning lines (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseWarningDoesNotEchoLine(t *testing.T) {
	_, warnings, err := Parse("f.env", strings.NewReader("hunter2-s3cret pasted here\n"))
	if err != nil || len(warnings) != 1 {
		t.Fatalf("err %v, warnings %v", err, warnings)
	}
	if strings.Contains(warnings[0].String(), "hunter2") {
		t.Errorf("warning echoes the line: %s", warnings[0])
	}
}

func TestParseAnnotationAttachesRef(t *testing.T) {
	entries, _, err := Parse("f", strings.NewReader("A=1\n# secretref: keyring://s/a\nB=2\nC=3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Ref != nil || entries[2].Ref != nil {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	if r := entries[1].Ref; r == nil || r.Scheme != "keyring" || r.Name != "a" {
		t.Errorf("entry B ref = %+v", entries[1].Ref)
	}
}

func TestParseInvalidRefDoesNotEchoText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"# secretref: hunter2-s3cret\nK=v\n", "invalid reference"},
		{"# secretref: nope://hunter2-s3cret/x\nK=v\n", "invalid reference: unknown scheme; supported: aws-sm, gcp-sm, keyring"},
		{"# secretref: hunter2-s3cret://x/y\nK=v\n", "invalid reference: unknown scheme; supported: aws-sm, gcp-sm, keyring"},
		{"# secretref: keyring://hunter2-s3cret\nK=v\n", "invalid reference (scheme keyring)"},
		{"# secretref: aws-sm://r/n?hunter2-s3cret=1\nK=v\n", "invalid reference (scheme aws-sm)"},
	}
	for _, tt := range tests {
		_, _, err := Parse("f.env", strings.NewReader(tt.in))
		if err == nil {
			t.Fatalf("%q: want error", tt.in)
		}
		if strings.Contains(err.Error(), "hunter2") {
			t.Errorf("error echoes pasted text: %v", err)
		}
		if !strings.HasSuffix(err.Error(), tt.want) {
			t.Errorf("error %q does not end with %q", err, tt.want)
		}
	}
}

func TestParseCRLFMultiline(t *testing.T) {
	in := "A=1\r\nB=\"x\r\ny\"\r\nC='p\r\nq'\r\n"
	entries, _, err := Parse("f", strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{entries[0].Value, entries[1].Value, entries[2].Value}
	want := []string{"1", "x\ny", "p\nq"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}
