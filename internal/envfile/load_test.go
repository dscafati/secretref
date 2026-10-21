package envfile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadOrder(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.env", "X=1\nY=2\n")
	b := write(t, dir, "b.env", "X=3\n")
	entries, err := Load(context.Background(), []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.File+":"+e.Key+"="+e.Value)
	}
	want := []string{a + ":X=1", a + ":Y=2", b + ":X=3"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("order (-want +got):\n%s", diff)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(context.Background(), []string{filepath.Join(t.TempDir(), "nope.env")}); err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestLoadParseErrorNamesFile(t *testing.T) {
	p := write(t, t.TempDir(), "bad.env", "1X=y\n")
	_, err := Load(context.Background(), []string{p})
	pe, ok := err.(*ParseError) //nolint:errorlint // Load returns *ParseError directly.
	if !ok || pe.File != p || pe.Line != 1 {
		t.Fatalf("err = %v, want ParseError for %s:1", err, p)
	}
}

func TestLoadHonorsCanceledContext(t *testing.T) {
	p := write(t, t.TempDir(), "a.env", "X=1\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Load(ctx, []string{p}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestDefaultFiles(t *testing.T) {
	dir := t.TempDir()
	if got := DefaultFiles(dir); len(got) != 0 {
		t.Errorf("empty dir: got %v", got)
	}
	local := write(t, dir, ".env.local", "A=1\n")
	if diff := cmp.Diff([]string{local}, DefaultFiles(dir)); diff != "" {
		t.Errorf("only .env.local (-want +got):\n%s", diff)
	}
	base := write(t, dir, ".env", "A=1\n")
	if diff := cmp.Diff([]string{base, local}, DefaultFiles(dir)); diff != "" {
		t.Errorf("both (-want +got):\n%s", diff)
	}
}

func TestDefaultFilesSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".env"), 0o750); err != nil {
		t.Fatal(err)
	}
	if got := DefaultFiles(dir); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}
