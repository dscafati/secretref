package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v3"

	"github.com/dscafati/secretref/internal/cli"
	"github.com/dscafati/secretref/internal/procexec"
	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/fake"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

const (
	canary = "SR_CANARY_7f3a9c"
	refB   = "keyring://app/b"
)

// started records one call to Replace or Spawn.
type started struct {
	how  string // "replace" or "spawn"
	argv []string
	env  []string
}

// harness is a fake set of Deps around a temporary working directory.
type harness struct {
	t        *testing.T
	dir      string
	env      map[string]string
	fake     *fake.Fake
	reg      provider.Registry
	stdin    string
	terminal bool
	password string
	getwdErr error

	replaceErr error
	spawnCode  int
	duringRun  func(s started) // runs inside Replace/Spawn
	calls      []started

	stdout, stderr bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	f := fake.New()
	f.Put(refB, canary)
	return &harness{
		t:    t,
		dir:  t.TempDir(),
		env:  map[string]string{"PATH": "/usr/bin"},
		fake: f,
		reg:  provider.Registry{"keyring": f, "aws-sm": f, "gcp-sm": f},
	}
}

func (h *harness) write(name, content string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir, name), []byte(content), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) deps() cli.Deps {
	return cli.Deps{
		Registry: h.reg,
		Writers:  map[string]provider.Writer{"keyring": h.fake},
		LookupEnv: func(k string) (string, bool) {
			v, ok := h.env[k]
			return v, ok
		},
		Environ: func() []string {
			var out []string
			for k, v := range h.env {
				out = append(out, k+"="+v)
			}
			slices.Sort(out)
			return out
		},
		Getwd: func() (string, error) {
			if h.getwdErr != nil {
				return "", h.getwdErr
			}
			return h.dir, nil
		},
		Stdin:        strings.NewReader(h.stdin),
		Stdout:       &h.stdout,
		Stderr:       &h.stderr,
		IsTerminal:   func() bool { return h.terminal },
		ReadPassword: func() ([]byte, error) { return []byte(h.password), nil },
		Replace: func(argv, env []string) error {
			s := started{"replace", argv, env}
			h.calls = append(h.calls, s)
			if h.duringRun != nil {
				h.duringRun(s)
			}
			return h.replaceErr
		},
		Spawn: func(_ context.Context, argv, env []string) (int, error) {
			s := started{"spawn", argv, env}
			h.calls = append(h.calls, s)
			if h.duringRun != nil {
				h.duringRun(s)
			}
			return h.spawnCode, h.replaceErr
		},
	}
}

func (h *harness) run(args ...string) int {
	h.t.Helper()
	h.stdout.Reset()
	h.stderr.Reset()
	code := cli.Execute(context.Background(), args, h.deps())
	for name, out := range map[string]string{"stdout": h.stdout.String(), "stderr": h.stderr.String()} {
		if strings.Contains(out, canary) {
			h.t.Errorf("%s leaked the canary:\n%s", name, out)
		}
	}
	return code
}

func (h *harness) expect(code int, args ...string) {
	h.t.Helper()
	if got := h.run(args...); got != code {
		h.t.Fatalf("secretref %q = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, got, code, h.stdout.String(), h.stderr.String())
	}
}

// directMode is how run starts a command outside compose mode on this OS.
func directMode() string {
	if runtime.GOOS == "windows" {
		return "spawn"
	}
	return "replace"
}

func envOf(env []string, key string) (string, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

const annotatedEnv = "A=1\n# secretref: " + refB + "\nB=placeholder\n"

func TestRunNeedsDashAndCommand(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv)
	for _, args := range [][]string{
		{"run", "tool"},
		{"run"},
		{"run", "--"},
		{"run", "tool", "--", "x"},
	} {
		h.expect(2, args...)
		if len(h.calls) != 0 {
			t.Fatalf("%q started a command", args)
		}
	}
	if !strings.Contains(h.stderr.String(), "secretref run -- ") {
		t.Errorf("usage hint missing: %s", h.stderr.String())
	}
}

func TestRunNoEnvFile(t *testing.T) {
	h := newHarness(t)
	h.expect(2, "run", "--", "tool")
	want := "secretref: no .env or .env.local found in " + h.dir + "\n"
	if h.stderr.String() != want {
		t.Errorf("stderr = %q, want %q", h.stderr.String(), want)
	}
}

func TestRunInjects(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv)
	h.write(".env.local", "C=${B}@host\n")
	h.expect(0, "run", "--", "tool", "arg with space")
	if len(h.calls) != 1 || h.calls[0].how != directMode() {
		t.Fatalf("calls = %v", h.calls)
	}
	c := h.calls[0]
	if diff := cmp.Diff([]string{"tool", "arg with space"}, c.argv); diff != "" {
		t.Errorf("argv (-want +got):\n%s", diff)
	}
	for k, want := range map[string]string{"A": "1", "B": canary, "C": canary + "@host", "PATH": "/usr/bin"} {
		if got, _ := envOf(c.env, k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if h.stdout.Len() != 0 || h.stderr.Len() != 0 {
		t.Errorf("run is not silent: stdout %q stderr %q", h.stdout.String(), h.stderr.String())
	}
}

func TestRunEnvironmentWins(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv)
	h.env["B"] = "from-env"
	h.expect(0, "run", "--", "tool")
	if got, _ := envOf(h.calls[0].env, "B"); got != "from-env" {
		t.Errorf("B = %q", got)
	}
	if calls := h.fake.Calls(); len(calls) != 0 {
		t.Errorf("resolved %v although the environment has B", calls)
	}
}

func TestRunEnvFileFlag(t *testing.T) {
	h := newHarness(t)
	h.write("custom.env", "X=custom\n")
	h.write(".env", "X=default\n")
	h.expect(0, "run", "--env-file", "custom.env", "--", "tool")
	if got, _ := envOf(h.calls[0].env, "X"); got != "custom" {
		t.Errorf("X = %q, want custom", got)
	}
	h.expect(2, "run", "--env-file", "missing.env", "--", "tool")
}

func TestRunFailure(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv+"# secretref: keyring://app/c\nC=x\n")
	h.fake.Fail(refB, fmt.Errorf("AWS error AccessDeniedException: %w", provider.ErrDenied))
	h.expect(3, "run", "--", "tool")
	if len(h.calls) != 0 {
		t.Fatal("command started despite a failure")
	}
	lines := strings.Split(strings.TrimSuffix(h.stderr.String(), "\n"), "\n")
	want := []string{
		"secretref: B  keyring://app/b  denied: AWS error AccessDeniedException",
		"secretref: C  keyring://app/c  not found",
	}
	if diff := cmp.Diff(want, lines); diff != "" {
		t.Errorf("stderr lines (-want +got):\n%s", diff)
	}
}

func TestRunAllowMissing(t *testing.T) {
	h := newHarness(t)
	h.write(".env", "# secretref: keyring://app/missing\nM=fallback\n")
	h.expect(0, "run", "--allow-missing", "--", "tool")
	if got, _ := envOf(h.calls[0].env, "M"); got != "fallback" {
		t.Errorf("M = %q, want fallback", got)
	}
	want := "secretref: warning: M  keyring://app/missing  not found; using the value from .env\n"
	if diff := cmp.Diff(want, h.stderr.String()); diff != "" {
		t.Errorf("stderr (-want +got):\n%s", diff)
	}
}

func TestRunAllowMissingWarningMatchesFailure(t *testing.T) {
	h := newHarness(t)
	if err := os.Mkdir(filepath.Join(h.dir, "conf"), 0o700); err != nil {
		t.Fatal(err)
	}
	h.write(filepath.Join("conf", "app.env"), "# secretref: keyring://app/b\nB=fallback\n")
	h.fake.Fail(refB, fmt.Errorf("AWS error AccessDeniedException: %w", provider.ErrDenied))
	h.expect(0, "run", "--allow-missing", "--env-file", filepath.Join("conf", "app.env"), "--", "tool")
	want := "secretref: warning: B  keyring://app/b  denied: AWS error AccessDeniedException; using the value from " +
		filepath.Join("conf", "app.env") + "\n"
	if diff := cmp.Diff(want, h.stderr.String()); diff != "" {
		t.Errorf("stderr (-want +got):\n%s", diff)
	}
}

func TestRunVerbose(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv)
	h.expect(0, "run", "--verbose", "--", "tool")
	if !strings.Contains(h.stderr.String(), "resolved B from "+refB) {
		t.Errorf("stderr = %q", h.stderr.String())
	}
	if strings.Contains(h.stderr.String(), "resolved A") {
		t.Errorf("logged a plain variable as resolved: %q", h.stderr.String())
	}
}

func TestRunCommandNotFound(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv)
	h.replaceErr = fmt.Errorf("tool: %w", procexec.ErrCommandNotFound)
	h.expect(127, "run", "--", "tool")
	if got := h.stderr.String(); got != "secretref: command not found: tool\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestRunStartError(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv)
	h.replaceErr = errors.New("exec format error")
	h.expect(1, "run", "--", "tool")
}

func TestRunCompose(t *testing.T) {
	h := newHarness(t)
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	h.write(".env", annotatedEnv)
	h.write("compose.yaml", "services:\n  web:\n    image: alpine\n    env_file: .env\n")
	h.spawnCode = 4
	var override string
	h.duringRun = func(s started) {
		v, _ := envOf(s.env, "COMPOSE_FILE")
		parts := strings.Split(v, string(os.PathListSeparator))
		override = parts[len(parts)-1]
		data, err := os.ReadFile(override) //nolint:gosec // G304: the override path the code under test created.
		if err != nil {
			t.Errorf("override not readable during run: %v", err)
		}
		if !strings.Contains(string(data), "B: ${B}") || strings.Contains(string(data), canary) {
			t.Errorf("override = %q", data)
		}
	}
	h.expect(4, "run", "--", "docker", "compose", "up")
	if len(h.calls) != 1 || h.calls[0].how != "spawn" {
		t.Fatalf("calls = %v", h.calls)
	}
	if got, _ := envOf(h.calls[0].env, "B"); got != canary {
		t.Errorf("compose env lacks B")
	}
	if _, err := os.Stat(override); !os.IsNotExist(err) {
		t.Errorf("override not removed: %v", err)
	}
}

func TestRunTimeout(t *testing.T) {
	h := newHarness(t)
	h.reg["keyring"] = slowProvider{}
	h.write(".env", annotatedEnv)
	h.expect(3, "run", "--timeout", "1ms", "--", "tool")
	if got := h.stderr.String(); got != "secretref: B  "+refB+"  unavailable: timed out after 1ms\n" {
		t.Errorf("stderr = %q", h.stderr.String())
	}
	h.expect(2, "run", "--timeout", "0s", "--", "tool")
}

type slowProvider struct{}

func (slowProvider) Resolve(ctx context.Context, _ ref.Ref) (secret.Secret, error) {
	<-ctx.Done()
	return secret.Secret{}, ctx.Err()
}

func TestRunDanglingAnnotation(t *testing.T) {
	h := newHarness(t)
	h.write(".env", "# secretref: "+refB+"\n\nB=x\n")
	h.expect(2, "run", "--", "tool")
	if !strings.Contains(h.stderr.String(), ".env:1:") {
		t.Errorf("stderr = %q, want file:line", h.stderr.String())
	}
}

func TestInternalError(t *testing.T) {
	h := newHarness(t)
	h.getwdErr = errors.New("getwd failed")
	h.expect(1, "run", "--", "tool")
	h.expect(1, "check")
}

func TestCheck(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv+"# secretref: keyring://app/c\nC=x\n")
	h.env["B"] = "set-in-env" // check ignores the environment
	h.expect(3, "check")
	want := "KEY  REF              STATUS\n" +
		"B    keyring://app/b  ok\n" +
		"C    keyring://app/c  not found\n"
	if diff := cmp.Diff(want, h.stdout.String()); diff != "" {
		t.Errorf("stdout (-want +got):\n%s", diff)
	}

	h.fake.Put("keyring://app/c", "v")
	h.expect(0, "check")
}

func TestSet(t *testing.T) {
	tests := []struct {
		name     string
		terminal bool
		password string
		stdin    string
		want     string
	}{
		{name: "terminal", terminal: true, password: canary, want: canary},
		{name: "stdin newline", stdin: canary + "\n", want: canary},
		{name: "stdin crlf", stdin: canary + "\r\n", want: canary},
		{name: "stdin trims once", stdin: canary + "\n\n", want: canary + "\n"},
		{name: "stdin no newline", stdin: canary, want: canary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.terminal, h.password, h.stdin = tt.terminal, tt.password, tt.stdin
			h.expect(0, "set", "keyring://app/new")
			r, _ := ref.Parse("keyring://app/new")
			got, err := h.fake.Resolve(context.Background(), r)
			if err != nil || string(got.Reveal()) != tt.want {
				t.Errorf("stored %q, %v; want %q", got.Reveal(), err, tt.want)
			}
			if !strings.HasSuffix(h.stderr.String(), "stored keyring://app/new\n") {
				t.Errorf("stderr = %q", h.stderr.String())
			}
			hasPrompt := strings.Contains(h.stderr.String(), "Value for keyring://app/new: ")
			if hasPrompt != tt.terminal {
				t.Errorf("prompt shown = %v, want %v", hasPrompt, tt.terminal)
			}
		})
	}
}

func TestSetRejects(t *testing.T) {
	h := newHarness(t)
	h.stdin = "v\n"
	h.expect(2, "set", "aws-sm://eu-west-1/x")
	h.expect(2, "set", "not-a-ref")
	h.expect(2, "set")
	h.stdin = "\n"
	h.expect(2, "set", "keyring://app/x")
}

func TestDelete(t *testing.T) {
	h := newHarness(t)
	h.expect(0, "delete", refB)
	r, _ := ref.Parse(refB)
	if _, err := h.fake.Resolve(context.Background(), r); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("still stored: %v", err)
	}
	h.expect(3, "delete", refB)
	if got := h.stderr.String(); got != "secretref: "+refB+": not found\n" {
		t.Errorf("stderr = %q", got)
	}
	h.expect(2, "delete", "gcp-sm://p/s")
}

func TestVersion(t *testing.T) {
	h := newHarness(t)
	h.expect(0, "version")
	if got := h.stdout.String(); got != "secretref dev\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestCompletion(t *testing.T) {
	h := newHarness(t)
	h.expect(0, "completion", "bash")
	if !strings.Contains(h.stdout.String(), "secretref") {
		t.Errorf("no completion script: %q", h.stdout.String())
	}
}

func TestUsageErrors(t *testing.T) {
	h := newHarness(t)
	h.expect(2, "nope")
	if !strings.Contains(h.stderr.String(), `unknown command "nope"`) {
		t.Errorf("stderr = %q", h.stderr.String())
	}
	h.expect(2, "run", "--bogus", "--", "tool")
	h.expect(2, "--bogus")
	h.expect(2, "version", "extra")
	h.expect(2, "completion", "bash", "extra") // cobra's own argument check
	h.expect(0)
}

func TestDescribeDropsRepeatedKind(t *testing.T) {
	h := newHarness(t)
	h.write(".env", "# secretref: keyring://app/x\nX=1\n")
	h.fake.Fail("keyring://app/x", provider.ErrUnavailable) // the bare sentinel
	h.expect(3, "check")
	if !strings.Contains(h.stdout.String(), "keyring://app/x  unavailable\n") {
		t.Errorf("stdout = %q", h.stdout.String())
	}
}

func TestRunComposeWarnsWhenNoServiceMatches(t *testing.T) {
	h := newHarness(t)
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	h.write(".env", annotatedEnv)
	h.write("compose.yaml", "services:\n  web:\n    image: alpine\n")
	h.expect(0, "run", "--", "docker", "compose", "up")
	want := "secretref: compose: no service uses .env; containers will see file values\n"
	if got := h.stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if len(h.calls) != 1 || h.calls[0].how != "spawn" {
		t.Fatalf("calls = %v", h.calls)
	}

	// No compose file at all warns the same way.
	if err := os.Remove(filepath.Join(h.dir, "compose.yaml")); err != nil {
		t.Fatal(err)
	}
	h.expect(0, "run", "--", "docker", "compose", "up")
	if got := h.stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestRunComposeWithDockerGlobalFlags(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv)
	h.expect(0, "run", "--", "docker", "--context", "remote", "compose", "up")
	if len(h.calls) != 1 || h.calls[0].how != "spawn" {
		t.Fatalf("calls = %v, want compose mode", h.calls)
	}
}

func TestRunVerboseSkipsFallback(t *testing.T) {
	h := newHarness(t)
	h.write(".env", annotatedEnv+"# secretref: keyring://app/missing\nM=x\n")
	h.expect(0, "run", "--verbose", "--allow-missing", "--", "tool")
	if strings.Contains(h.stderr.String(), "resolved M") || !strings.Contains(h.stderr.String(), "resolved B") {
		t.Errorf("stderr = %q", h.stderr.String())
	}
}

func TestNewRootHasCommands(t *testing.T) {
	root := cli.NewRoot(cli.Deps{Stdout: io.Discard, Stderr: io.Discard})
	var names []string
	for _, c := range root.Commands() {
		names = append(names, c.Name())
	}
	for _, want := range []string{"run", "check", "set", "delete", "version"} {
		if !slices.Contains(names, want) {
			t.Errorf("missing command %s in %v", want, names)
		}
	}
}

// composeOverride runs secretref with args in compose mode and returns the
// override's services, each mapped to its environment keys.
func composeOverride(h *harness, args ...string) map[string][]string {
	h.t.Helper()
	tmp := h.t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		h.t.Setenv(k, tmp)
	}
	got := map[string][]string{}
	h.duringRun = func(s started) {
		v, _ := envOf(s.env, "COMPOSE_FILE")
		parts := strings.Split(v, string(os.PathListSeparator))
		data, err := os.ReadFile(parts[len(parts)-1])
		if err != nil {
			h.t.Fatalf("override not readable: %v", err)
		}
		var o struct {
			Services map[string]struct {
				Environment map[string]string `yaml:"environment"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(data, &o); err != nil {
			h.t.Fatalf("override is not YAML: %v", err)
		}
		for name, svc := range o.Services {
			for k := range svc.Environment {
				got[name] = append(got[name], k)
			}
			slices.Sort(got[name])
		}
	}
	h.expect(0, args...)
	return got
}

func TestRunComposeOverridesResolvedKeysOnly(t *testing.T) {
	// Plain keys (A) and keys kept from the file by --allow-missing (M) stay
	// with compose's own env_file handling.
	h := newHarness(t)
	h.write(".env", annotatedEnv+"# secretref: keyring://app/missing\nM=x\n")
	h.write("compose.yaml", "services:\n  web:\n    image: alpine\n    env_file: .env\n")
	got := composeOverride(h, "run", "--allow-missing", "--", "docker", "compose", "up")
	want := map[string][]string{"web": {"B"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("override services (-want +got):\n%s", diff)
	}
}

func TestRunComposeServiceGetsKeysFromItsOwnEnvFiles(t *testing.T) {
	// B's final value comes from .env.local, which worker does not list, so
	// worker keeps compose's reading of .env for B.
	h := newHarness(t)
	h.fake.Put("keyring://app/c", canary)
	h.fake.Put("keyring://app/d", canary)
	h.write(".env", annotatedEnv+"# secretref: keyring://app/c\nC=placeholder\n")
	h.write(".env.local", "# secretref: keyring://app/d\nB=local\n")
	h.write("compose.yaml", "services:\n"+
		"  web:\n    image: alpine\n    env_file: [.env, .env.local]\n"+
		"  worker:\n    image: alpine\n    env_file: .env\n")
	got := composeOverride(h, "run", "--", "docker", "compose", "up")
	want := map[string][]string{"web": {"B", "C"}, "worker": {"C"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("override services (-want +got):\n%s", diff)
	}
}

func TestRunComposeNoWarningWhenServiceSetsTheKey(t *testing.T) {
	h := newHarness(t)
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	h.write(".env", annotatedEnv)
	h.write("compose.yaml", "services:\n  web:\n    image: alpine\n    env_file: .env\n    environment: [B=db]\n")
	h.expect(0, "run", "--", "docker", "compose", "up")
	if got := h.stderr.String(); got != "" {
		t.Errorf("stderr = %q, want nothing", got)
	}
	if v, ok := envOf(h.calls[0].env, "COMPOSE_FILE"); ok {
		t.Errorf("COMPOSE_FILE = %q, want unset", v)
	}
}

func TestRunComposeWarnsAboutKeysComposeCannotSubstitute(t *testing.T) {
	h := newHarness(t)
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	h.write(".env", "# secretref: "+refB+"\na.b=placeholder\n")
	h.write("compose.yaml", "services:\n  web:\n    image: alpine\n    env_file: .env\n")
	h.expect(0, "run", "--", "docker", "compose", "up")
	want := "secretref: compose: warning: a.b is not a name compose can substitute; containers will see the value from env_file\n"
	if got := h.stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}
