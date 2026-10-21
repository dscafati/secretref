package resolve_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dscafati/secretref/internal/envfile"
	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/fake"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/resolve"
	"github.com/dscafati/secretref/internal/secret"
)

func mustRef(t *testing.T, s string) *ref.Ref {
	t.Helper()
	r, err := ref.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return &r
}

func noEnv(string) (string, bool) { return "", false }

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func newResolver(f *fake.Fake, env func(string) (string, bool), opts resolve.Options) *resolve.Resolver {
	return resolve.New(provider.Registry{"keyring": f, "aws-sm": f}, env, opts)
}

func values(res resolve.Result) map[string]string {
	m := map[string]string{}
	for _, v := range res.Vars {
		m[v.Key] = string(v.Value.Reveal())
	}
	return m
}

func plain(file, key, val string) envfile.Entry {
	return envfile.Entry{File: file, Key: key, Value: val}
}

func TestLaterFileWins(t *testing.T) {
	res := newResolver(fake.New(), noEnv, resolve.Options{}).Resolve(context.Background(), []envfile.Entry{
		plain(".env", "A", "1"), plain(".env", "B", "b"), plain(".env.local", "A", "2"),
	})
	if len(res.Vars) != 2 || res.Vars[0].Key != "A" || res.Vars[1].Key != "B" {
		t.Fatalf("Vars = %+v, want A then B", res.Vars)
	}
	if res.Vars[0].File != ".env.local" || string(res.Vars[0].Value.Reveal()) != "2" {
		t.Errorf("A = %+v", res.Vars[0])
	}
}

func TestLocalPlainOverridesAnnotated(t *testing.T) {
	f := fake.New()
	f.Put("keyring://s/a", "secret")
	res := newResolver(f, noEnv, resolve.Options{}).Resolve(context.Background(), []envfile.Entry{
		{File: ".env", Key: "A", Value: "fb", Ref: mustRef(t, "keyring://s/a")},
		plain(".env.local", "A", "x"),
	})
	if got := values(res)["A"]; got != "x" {
		t.Errorf("A = %q, want x", got)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("Calls = %v, want none", f.Calls())
	}
}

func TestEnvWins(t *testing.T) {
	f := fake.New()
	f.Put("keyring://s/a", "secret")
	entries := []envfile.Entry{{File: ".env", Key: "A", Value: "fb", Ref: mustRef(t, "keyring://s/a")}}
	env := envOf(map[string]string{"A": "fromenv"})

	res := newResolver(f, env, resolve.Options{}).Resolve(context.Background(), entries)
	if len(res.Vars) != 1 || !res.Vars[0].FromEnv || values(res)["A"] != "fromenv" {
		t.Fatalf("Vars = %+v", res.Vars)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("resolved despite env: %v", f.Calls())
	}
	if got := res.Environ([]string{"A=fromenv"}); !slices.Equal(got, []string{"A=fromenv"}) {
		t.Errorf("Environ = %v", got)
	}

	res = newResolver(f, env, resolve.Options{IgnoreEnv: true}).Resolve(context.Background(), entries)
	if res.Vars[0].FromEnv || values(res)["A"] != "secret" {
		t.Errorf("IgnoreEnv Vars = %+v", res.Vars)
	}
}

func TestFailuresCollected(t *testing.T) {
	f := fake.New()
	f.Put("keyring://s/ok", "v")
	f.Fail("keyring://s/b", fmt.Errorf("b: %w", provider.ErrDenied))
	entries := []envfile.Entry{
		{File: ".env.local", Key: "Z", Value: "zf", Ref: mustRef(t, "keyring://s/b")},
		{File: ".env", Key: "OK", Value: "", Ref: mustRef(t, "keyring://s/ok")},
		{File: ".env", Key: "A", Value: "af", Ref: mustRef(t, "keyring://s/missing")},
	}
	res := newResolver(f, noEnv, resolve.Options{}).Resolve(context.Background(), entries)
	if len(res.Failures) != 2 || res.Failures[0].Key != "A" || res.Failures[1].Key != "Z" {
		t.Fatalf("Failures = %+v", res.Failures)
	}
	if !errors.Is(res.Failures[0].Err, provider.ErrNotFound) || !errors.Is(res.Failures[1].Err, provider.ErrDenied) {
		t.Errorf("kinds: %v / %v", res.Failures[0].Err, res.Failures[1].Err)
	}
	if res.Failures[0].Ref.Raw != "keyring://s/missing" {
		t.Errorf("Ref = %v", res.Failures[0].Ref)
	}
	if diff := cmp.Diff(map[string]string{"OK": "v"}, values(res)); diff != "" {
		t.Errorf("Vars (-want +got):\n%s", diff)
	}

	res = newResolver(f, noEnv, resolve.Options{AllowMissing: true}).Resolve(context.Background(), entries)
	if len(res.Failures) != 0 || len(res.Warnings) != 2 {
		t.Fatalf("Failures=%v Warnings=%v", res.Failures, res.Warnings)
	}
	if diff := cmp.Diff(map[string]string{"Z": "zf", "OK": "v", "A": "af"}, values(res)); diff != "" {
		t.Errorf("Vars (-want +got):\n%s", diff)
	}
	type warning struct{ Key, Ref, Kind, File string }
	var gotWarnings []warning
	for _, w := range res.Warnings {
		gotWarnings = append(gotWarnings, warning{w.Key, w.Ref.Raw, provider.Kind(w.Err), w.File})
	}
	wantWarnings := []warning{
		{"A", "keyring://s/missing", "not found", ".env"},
		{"Z", "keyring://s/b", "denied", ".env.local"},
	}
	if diff := cmp.Diff(wantWarnings, gotWarnings); diff != "" {
		t.Errorf("Warnings (-want +got):\n%s", diff)
	}
	fellBack := map[string]bool{}
	for _, v := range res.Vars {
		fellBack[v.Key] = v.FellBack
	}
	if diff := cmp.Diff(map[string]bool{"Z": true, "OK": false, "A": true}, fellBack); diff != "" {
		t.Errorf("FellBack (-want +got):\n%s", diff)
	}
}

type blocking struct {
	cur, max atomic.Int32
	release  chan struct{}
}

func (b *blocking) Resolve(ctx context.Context, _ ref.Ref) (secret.Secret, error) {
	n := b.cur.Add(1)
	defer b.cur.Add(-1)
	for {
		m := b.max.Load()
		if n <= m || b.max.CompareAndSwap(m, n) {
			break
		}
	}
	select {
	case <-b.release:
	case <-ctx.Done():
		return secret.Secret{}, ctx.Err()
	}
	return secret.New([]byte("v")), nil
}

func TestConcurrencyLimit(t *testing.T) {
	b := &blocking{release: make(chan struct{})}
	var entries []envfile.Entry
	for i := range 12 {
		entries = append(entries, envfile.Entry{File: ".env", Key: fmt.Sprintf("K%d", i), Ref: mustRef(t, "keyring://s/a")})
	}
	r := resolve.New(provider.Registry{"keyring": b}, noEnv, resolve.Options{Concurrency: 3})
	done := make(chan resolve.Result)
	go func() { done <- r.Resolve(context.Background(), entries) }()

	deadline := time.Now().Add(2 * time.Second)
	for b.cur.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(30 * time.Millisecond)
	close(b.release)
	res := <-done
	if got := b.max.Load(); got != 3 {
		t.Errorf("max in flight = %d, want 3", got)
	}
	if len(res.Vars) != 12 {
		t.Errorf("Vars = %d, want 12", len(res.Vars))
	}
}

func TestTimeout(t *testing.T) {
	b := &blocking{release: make(chan struct{})}
	r := resolve.New(provider.Registry{"keyring": b}, noEnv, resolve.Options{Timeout: 50 * time.Millisecond})
	start := time.Now()
	res := r.Resolve(context.Background(), []envfile.Entry{{File: ".env", Key: "A", Ref: mustRef(t, "keyring://s/a")}})
	if el := time.Since(start); el > 150*time.Millisecond {
		t.Errorf("took %v", el)
	}
	if len(res.Failures) != 1 {
		t.Fatalf("Failures = %+v", res.Failures)
	}
	err := res.Failures[0].Err
	if !errors.Is(err, provider.ErrUnavailable) || !strings.Contains(err.Error(), "timed out after 50ms") {
		t.Errorf("err = %v", err)
	}
}

func TestUnknownSchemeIsInvalid(t *testing.T) {
	r := resolve.New(provider.Registry{}, noEnv, resolve.Options{})
	res := r.Resolve(context.Background(), []envfile.Entry{{File: ".env", Key: "A", Ref: mustRef(t, "keyring://s/a")}})
	if len(res.Failures) != 1 || !errors.Is(res.Failures[0].Err, provider.ErrInvalid) {
		t.Fatalf("Failures = %+v", res.Failures)
	}
}

func TestFieldExtraction(t *testing.T) {
	f := fake.New()
	f.Put("aws-sm://r/db#password", `{"password":"p"}`)
	f.Put("aws-sm://r/db#nope", `{"password":"p"}`)
	res := newResolver(f, noEnv, resolve.Options{}).Resolve(context.Background(), []envfile.Entry{
		{File: ".env", Key: "A", Ref: mustRef(t, "aws-sm://r/db#password")},
		{File: ".env", Key: "B", Ref: mustRef(t, "aws-sm://r/db#nope")},
	})
	if values(res)["A"] != "p" {
		t.Errorf("A = %q", values(res)["A"])
	}
	if len(res.Failures) != 1 || res.Failures[0].Key != "B" || !errors.Is(res.Failures[0].Err, provider.ErrInvalid) {
		t.Errorf("Failures = %+v", res.Failures)
	}
}

func TestExpand(t *testing.T) {
	f := fake.New()
	f.Put("keyring://s/pw", "real")
	f.Put("keyring://s/tricky", "${B}$B")
	env := envOf(map[string]string{"HOME_X": "/h", "FROMENV": "${NOT}"})
	entries := []envfile.Entry{
		{File: ".env", Key: "URL", Value: "pg://u:${PW}@h/$DB", Quote: envfile.QuoteDouble},
		{File: ".env", Key: "PW", Ref: mustRef(t, "keyring://s/pw")},
		{File: ".env", Key: "DB", Value: "app"},
		{File: ".env", Key: "B", Value: "bee"},
		{File: ".env", Key: "TRICKY", Ref: mustRef(t, "keyring://s/tricky")},
		{File: ".env", Key: "SINGLE", Value: "${DB} \\$X", Quote: envfile.QuoteSingle},
		{File: ".env", Key: "ESC", Value: `\$DB`, Quote: envfile.QuoteDouble},
		{File: ".env", Key: "ESCU", Value: `\$DB`},
		{File: ".env", Key: "DEF", Value: "${NOPE:-d}-${NOPE}-"},
		{File: ".env", Key: "ENV", Value: "$HOME_X/${FROMENV}"},
		{File: ".env", Key: "CHAIN", Value: "${URL2}"},
		{File: ".env", Key: "URL2", Value: "[${DB}]"},
		{File: ".env", Key: "CYC", Value: "${CYC}x"},
	}
	res := newResolver(f, env, resolve.Options{}).Resolve(context.Background(), entries)
	want := map[string]string{
		"URL":    "pg://u:real@h/app",
		"PW":     "real",
		"DB":     "app",
		"B":      "bee",
		"TRICKY": "${B}$B",
		"SINGLE": "${DB} \\$X",
		"ESC":    "$DB",
		"ESCU":   "$DB",
		"DEF":    "d--",
		"ENV":    "/h/${NOT}",
		"CHAIN":  "[app]",
		"URL2":   "[app]",
		"CYC":    "x",
	}
	if diff := cmp.Diff(want, values(res)); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestExpandSeesEnvOverride(t *testing.T) {
	env := envOf(map[string]string{"DB": "envdb"})
	res := newResolver(fake.New(), env, resolve.Options{}).Resolve(context.Background(), []envfile.Entry{
		plain(".env", "DB", "filedb"), plain(".env", "X", "$DB"),
	})
	if got := values(res)["X"]; got != "envdb" {
		t.Errorf("X = %q, want envdb", got)
	}
}

func TestVarOrderFirstAppearance(t *testing.T) {
	res := newResolver(fake.New(), noEnv, resolve.Options{}).Resolve(context.Background(), []envfile.Entry{
		plain(".env", "B", "1"), plain(".env", "A", "1"), plain(".env.local", "B", "2"),
	})
	var keys []string
	for _, v := range res.Vars {
		keys = append(keys, v.Key)
	}
	if !slices.Equal(keys, []string{"B", "A"}) {
		t.Errorf("order = %v", keys)
	}
}

func TestEnviron(t *testing.T) {
	f := fake.New()
	f.Put("keyring://s/a", "sec")
	env := envOf(map[string]string{"E": "e"})
	res := newResolver(f, env, resolve.Options{}).Resolve(context.Background(), []envfile.Entry{
		plain(".env", "E", "x"), plain(".env", "P", "p"), {File: ".env", Key: "S", Ref: mustRef(t, "keyring://s/a")},
	})
	got := res.Environ([]string{"Z=1", "E=e"})
	if want := []string{"Z=1", "E=e", "P=p", "S=sec"}; !slices.Equal(got, want) {
		t.Errorf("Environ = %v, want %v", got, want)
	}
}

func TestNoLeakInFormatting(t *testing.T) {
	f := fake.New()
	f.Put("keyring://s/a", "hunter2-canary")
	res := newResolver(f, noEnv, resolve.Options{}).Resolve(context.Background(), []envfile.Entry{
		{File: ".env", Key: "S", Ref: mustRef(t, "keyring://s/a")},
	})
	if out := fmt.Sprintf("%v %+v %#v", res, res, res); strings.Contains(out, "canary") {
		t.Errorf("leak: %s", out)
	}
}

func TestCycleOrderFollowsFile(t *testing.T) {
	for _, tt := range []struct {
		first, second string
		want          map[string]string
	}{
		{"A", "B", map[string]string{"A": "ab", "B": "b"}},
		{"B", "A", map[string]string{"A": "a", "B": "ba"}},
	} {
		vals := map[string]string{"A": "a${B}", "B": "b${A}"}
		res := newResolver(fake.New(), noEnv, resolve.Options{}).Resolve(context.Background(), []envfile.Entry{
			plain(".env", tt.first, vals[tt.first]), plain(".env", tt.second, vals[tt.second]),
		})
		if diff := cmp.Diff(tt.want, values(res)); diff != "" {
			t.Errorf("%s first (-want +got):\n%s", tt.first, diff)
		}
	}
}

func TestWarningsOrderedByKey(t *testing.T) {
	f := fake.New()
	res := newResolver(f, noEnv, resolve.Options{AllowMissing: true}).Resolve(context.Background(), []envfile.Entry{
		{File: ".env", Key: "A!", Ref: mustRef(t, "keyring://s/1")},
		{File: ".env", Key: "A", Ref: mustRef(t, "keyring://s/2")},
	})
	if len(res.Warnings) != 2 || res.Warnings[0].Key != "A" || res.Warnings[1].Key != "A!" {
		t.Errorf("Warnings = %+v", res.Warnings)
	}
}

type deaf struct{ sleep time.Duration }

func (d deaf) Resolve(context.Context, ref.Ref) (secret.Secret, error) {
	time.Sleep(d.sleep)
	return secret.New([]byte("late")), nil
}

func TestProviderIgnoringContext(t *testing.T) {
	r := resolve.New(provider.Registry{"keyring": deaf{sleep: 600 * time.Millisecond}}, noEnv,
		resolve.Options{Timeout: 100 * time.Millisecond})
	start := time.Now()
	res := r.Resolve(context.Background(), []envfile.Entry{
		{File: ".env", Key: "A", Ref: mustRef(t, "keyring://s/a")},
		{File: ".env", Key: "B", Ref: mustRef(t, "keyring://s/b")},
	})
	if el := time.Since(start); el > 300*time.Millisecond {
		t.Errorf("Resolve took %v", el)
	}
	if len(res.Failures) != 2 || len(res.Vars) != 0 {
		t.Fatalf("Failures=%+v Vars=%+v", res.Failures, res.Vars)
	}
	for _, f := range res.Failures {
		if !errors.Is(f.Err, provider.ErrUnavailable) || !strings.Contains(f.Err.Error(), "timed out after 100ms") {
			t.Errorf("err = %v", f.Err)
		}
	}
	time.Sleep(600 * time.Millisecond) // let late workers finish under -race
}

func TestParentCancel(t *testing.T) {
	b := &blocking{release: make(chan struct{})}
	r := resolve.New(provider.Registry{"keyring": b}, noEnv, resolve.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	res := r.Resolve(ctx, []envfile.Entry{{File: ".env", Key: "A", Ref: mustRef(t, "keyring://s/a")}})
	if len(res.Failures) != 1 {
		t.Fatalf("Failures = %+v", res.Failures)
	}
	err := res.Failures[0].Err
	if !errors.Is(err, provider.ErrUnavailable) || !strings.Contains(err.Error(), "canceled") {
		t.Errorf("err = %v", err)
	}
}
