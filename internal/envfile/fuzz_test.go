package envfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var validKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

func FuzzParse(f *testing.F) {
	seeds, _ := filepath.Glob("testdata/*.env")
	for _, p := range seeds {
		b, err := os.ReadFile(p) //nolint:gosec // Fixed testdata glob.
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
		f.Add(bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n")))
	}
	f.Add([]byte("# secretref: keyring://a/b\nK=\"unterminated"))
	f.Add([]byte("A=1\r\nB='x\r\ny'\r\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		const name = "fuzz.env"
		entries, err := Parse(name, bytes.NewReader(data))
		lines := bytes.Count(data, []byte("\n")) + 1

		// Compare LF and CRLF forms of the same CR-free text.
		lf := bytes.ReplaceAll(data, []byte("\r"), nil)
		lfEntries, lfErr := Parse(name, bytes.NewReader(lf))
		crlf, crlfErr := Parse(name, bytes.NewReader(bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))))
		if (lfErr == nil) != (crlfErr == nil) {
			t.Fatalf("LF err %v, CRLF err %v", lfErr, crlfErr)
		}
		if lfErr == nil {
			if diff := cmp.Diff(string(render(lfEntries)), string(render(crlf))); diff != "" {
				t.Fatalf("CRLF differs from LF:\n%s", diff)
			}
		}
		if err != nil {
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("error %T is not *ParseError", err)
			}
			if pe.Line < 1 || pe.Line > lines {
				t.Fatalf("error line %d outside 1..%d", pe.Line, lines)
			}
			if pe.File != name {
				t.Fatalf("error file %q", pe.File)
			}
			return
		}
		prev := 0
		for _, e := range entries {
			if !validKey.MatchString(e.Key) {
				t.Fatalf("invalid key %q", e.Key)
			}
			if e.Line <= prev {
				t.Fatalf("line %d not after %d", e.Line, prev)
			}
			prev = e.Line
			if e.File != name {
				t.Fatalf("File = %q", e.File)
			}
			if strings.ContainsRune(e.Key, '\n') {
				t.Fatal("newline in key")
			}
		}
	})
}
