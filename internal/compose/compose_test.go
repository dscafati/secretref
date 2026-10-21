package compose_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v3"

	"github.com/dscafati/secretref/internal/compose"
)

const canary = "SR_CANARY_7f3a9c"

func TestDetect(t *testing.T) {
	tests := []struct {
		argv []string
		want bool
	}{
		{[]string{"docker", "compose", "up"}, true},
		{[]string{"/usr/bin/docker", "compose", "up"}, true},
		{[]string{"docker.exe", "compose", "up"}, true},
		{[]string{`C:\Program Files\Docker\docker.exe`, "compose", "up"}, true},
		{[]string{"docker", "--context", "remote", "compose", "up"}, true},
		{[]string{"docker", "--context=remote", "compose", "up"}, true},
		{[]string{"docker", "-c", "remote", "-D", "compose", "up"}, true},
		{[]string{"docker", "-H", "tcp://h:2376", "--tlsverify", "--tlscacert", "ca.pem", "--tlscert=c.pem", "--tlskey", "k.pem", "compose", "up"}, true},
		{[]string{"docker", "--host", "unix:///s", "--config", "/cfg", "-l", "debug", "--log-level", "info", "--debug", "--tls", "compose", "up"}, true},
		{[]string{"docker", "--context", "compose", "up"}, false}, // "compose" is the context name
		{[]string{"docker", "--unknown", "compose", "up"}, false},
		{[]string{"docker", "run", "alpine"}, false},
		{[]string{"docker-compose", "up"}, false},
		{[]string{"docker"}, false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := compose.Detect(tt.argv); got != tt.want {
			t.Errorf("Detect(%q) = %v, want %v", tt.argv, got, tt.want)
		}
	}
}

// project creates dir with the given files and returns dir.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const simpleCompose = "services:\n  web:\n    image: alpine\n    env_file: .env\n"

// useTempDir points os.TempDir at a fresh directory and returns it.
func useTempDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, tmp)
	}
	return tmp
}

// prepare runs Prepare with environ as both the parent and the child
// environment, and returns the plan and the override path ("" if none was
// written).
func prepare(t *testing.T, argv, environ []string, keys map[string][]string, cwd string) (compose.Plan, string) {
	t.Helper()
	return prepareInput(t, compose.Input{Argv: argv, Environ: environ, Parent: environ, Keys: keys, Cwd: cwd})
}

// prepareInput runs Prepare on in and returns the plan and the override path
// ("" if none was written).
func prepareInput(t *testing.T, in compose.Input) (compose.Plan, string) {
	t.Helper()
	tmp := useTempDir(t)
	plan, err := compose.Prepare(context.Background(), in)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	t.Cleanup(func() { _ = plan.Cleanup() })
	matches, err := filepath.Glob(filepath.Join(tmp, "secretref-compose-override-*.yaml"))
	if err != nil || len(matches) > 1 {
		t.Fatalf("override files: %v, %v", matches, err)
	}
	if len(matches) == 0 {
		return plan, ""
	}
	return plan, matches[0]
}

func envGet(env []string, key string) (string, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

type override struct {
	Services map[string]struct {
		Environment map[string]string `yaml:"environment"`
	} `yaml:"services"`
}

func readOverride(t *testing.T, path string) (string, override) {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // G304: the override path the code under test created.
	if err != nil {
		t.Fatal(err)
	}
	var o override
	if err := yaml.Unmarshal(data, &o); err != nil {
		t.Fatalf("override is not YAML: %v\n%s", err, data)
	}
	return string(data), o
}

func TestEnvFileForms(t *testing.T) {
	forms, err := os.ReadFile(filepath.Join("testdata", "env_file_forms.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := project(t, map[string]string{"compose.yaml": string(forms)})
	keys := map[string][]string{
		filepath.Join(dir, ".env"):       {"DB_PASSWORD", "API_KEY"},
		filepath.Join(dir, ".env.local"): {"LOCAL_ONLY"},
	}
	environ := []string{"PATH=/bin", "DB_PASSWORD=" + canary, "API_KEY=" + canary, "LOCAL_ONLY=x"}
	_, path := prepare(t, []string{"docker", "compose", "up"}, environ, keys, dir)

	text, o := readOverride(t, path)
	if strings.Contains(text, canary) {
		t.Fatalf("override contains a value:\n%s", text)
	}
	got := map[string]map[string]string{}
	for name, svc := range o.Services {
		got[name] = svc.Environment
	}
	both := map[string]string{"API_KEY": "${API_KEY}", "DB_PASSWORD": "${DB_PASSWORD}"}
	want := map[string]map[string]string{
		"string_form": both,
		"object_form": both,
		"list_form":   {"LOCAL_ONLY": "${LOCAL_ONLY}"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("services (-want +got):\n%s", diff)
	}
}

func TestOverrideIsDeterministicAndLiteral(t *testing.T) {
	dir := project(t, map[string]string{"compose.yaml": simpleCompose})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"B", "A"}}
	_, path := prepare(t, []string{"docker", "compose", "up"}, nil, keys, dir)
	text, _ := readOverride(t, path)
	want := "services:\n    web:\n        environment:\n            A: ${A}\n            B: ${B}\n"
	if text != want {
		t.Errorf("override =\n%s\nwant\n%s", text, want)
	}
	if strings.Contains(text, "$$") {
		t.Error("override escapes $; compose would not interpolate")
	}
}

func TestOverrideFile(t *testing.T) {
	dir := project(t, map[string]string{"compose.yaml": simpleCompose})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	plan, path := prepare(t, []string{"docker", "compose", "up"}, nil, keys, dir)

	if plan.Override != path {
		t.Errorf("Plan.Override = %q, want %q", plan.Override, path)
	}
	if base := filepath.Base(path); !strings.HasPrefix(base, "secretref-compose-override-") || !strings.HasSuffix(base, ".yaml") {
		t.Errorf("override name = %s", base)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	if err := plan.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("override still exists after Cleanup: %v", err)
	}
}

func TestComposeFileWithoutFlags(t *testing.T) {
	sep := string(os.PathListSeparator)
	dir := project(t, map[string]string{
		"compose.yaml":          simpleCompose,
		"compose.override.yaml": "services:\n  web:\n    image: busybox\n",
	})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	argv := []string{"docker", "compose", "up", "-d"}
	plan, path := prepare(t, argv, []string{"X=1"}, keys, dir)

	if diff := cmp.Diff(argv, plan.Args); diff != "" {
		t.Errorf("args changed (-want +got):\n%s", diff)
	}
	got, _ := envGet(plan.Env, "COMPOSE_FILE")
	want := filepath.Join(dir, "compose.yaml") + sep + filepath.Join(dir, "compose.override.yaml") + sep + path
	if got != want {
		t.Errorf("COMPOSE_FILE = %q, want %q", got, want)
	}
	if v, _ := envGet(plan.Env, "X"); v != "1" {
		t.Error("environment not preserved")
	}
}

func TestComposeFileFromEnvironment(t *testing.T) {
	dir := project(t, map[string]string{"sub/app.yaml": simpleCompose})
	sep := string(os.PathListSeparator)
	// Paths inside compose files are relative to the first file's directory.
	keys := map[string][]string{filepath.Join(dir, "sub", ".env"): {"A"}}
	orig := filepath.Join("sub", "app.yaml")
	plan, path := prepare(t, []string{"docker", "compose", "up"}, []string{"COMPOSE_FILE=" + orig}, keys, dir)
	got, _ := envGet(plan.Env, "COMPOSE_FILE")
	if want := orig + sep + path; got != want {
		t.Errorf("COMPOSE_FILE = %q, want %q", got, want)
	}
	n := 0
	for _, kv := range plan.Env {
		if strings.HasPrefix(kv, "COMPOSE_FILE=") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("COMPOSE_FILE appears %d times", n)
	}
}

func TestComposePathSeparator(t *testing.T) {
	dir := project(t, map[string]string{"a.yaml": simpleCompose, "b.yaml": "services: {}\n"})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	environ := []string{"COMPOSE_PATH_SEPARATOR=,", "COMPOSE_FILE=a.yaml,b.yaml"}
	plan, path := prepare(t, []string{"docker", "compose", "up"}, environ, keys, dir)
	got, _ := envGet(plan.Env, "COMPOSE_FILE")
	if want := "a.yaml,b.yaml," + path; got != want {
		t.Errorf("COMPOSE_FILE = %q, want %q", got, want)
	}
}

func TestDefaultFileOrder(t *testing.T) {
	// compose.yaml wins over docker-compose.yml. Like compose-go, any
	// override name in the same directory joins, whatever its stem.
	dir := project(t, map[string]string{
		"compose.yaml":                simpleCompose,
		"docker-compose.yml":          "services: {}\n",
		"docker-compose.override.yml": "services: {}\n",
	})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	plan, path := prepare(t, []string{"docker", "compose", "up"}, nil, keys, dir)
	got, _ := envGet(plan.Env, "COMPOSE_FILE")
	want := filepath.Join(dir, "compose.yaml") + string(os.PathListSeparator) +
		filepath.Join(dir, "docker-compose.override.yml") + string(os.PathListSeparator) + path
	if got != want {
		t.Errorf("COMPOSE_FILE = %q, want %q", got, want)
	}

	dir = project(t, map[string]string{
		"docker-compose.yml":          simpleCompose,
		"docker-compose.override.yml": "services: {}\n",
	})
	keys = map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	plan, path = prepare(t, []string{"docker", "compose", "up"}, nil, keys, dir)
	got, _ = envGet(plan.Env, "COMPOSE_FILE")
	want = filepath.Join(dir, "docker-compose.yml") + string(os.PathListSeparator) +
		filepath.Join(dir, "docker-compose.override.yml") + string(os.PathListSeparator) + path
	if got != want {
		t.Errorf("COMPOSE_FILE = %q, want %q", got, want)
	}
}

func TestFileFlags(t *testing.T) {
	dir := project(t, map[string]string{"one.yaml": simpleCompose, "two.yaml": "services: {}\n"})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	const o = "OVERRIDE"
	// The override goes right after the user's last -f, before the
	// subcommand, so the subcommand's own -f (logs --follow) stays untouched.
	tests := map[string]struct{ argv, want []string }{
		"short": {
			argv: []string{"docker", "compose", "-f", "one.yaml", "-f", "two.yaml", "logs", "-f", "web"},
			want: []string{"docker", "compose", "-f", "one.yaml", "-f", "two.yaml", "-f", o, "logs", "-f", "web"},
		},
		"long": {
			argv: []string{"docker", "compose", "--file", "one.yaml", "--file=two.yaml", "logs", "-f"},
			want: []string{"docker", "compose", "--file", "one.yaml", "--file=two.yaml", "-f", o, "logs", "-f"},
		},
		"glued": {
			argv: []string{"docker", "compose", "-fone.yaml", "-p", "proj", "-f=two.yaml", "up"},
			want: []string{"docker", "compose", "-fone.yaml", "-p", "proj", "-f=two.yaml", "-f", o, "up"},
		},
		"docker global flags": {
			argv: []string{"docker", "--context", "c", "compose", "-f", "one.yaml", "up"},
			want: []string{"docker", "--context", "c", "compose", "-f", "one.yaml", "-f", o, "up"},
		},
		"value flags after": {
			argv: []string{"docker", "compose", "--project-name", "p", "-f", "one.yaml", "--profile", "x", "up"},
			want: []string{"docker", "compose", "--project-name", "p", "-f", "one.yaml", "-f", o, "--profile", "x", "up"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			plan, path := prepare(t, tt.argv, []string{"COMPOSE_FILE=ignored.yaml"}, keys, dir)
			want := slices.Clone(tt.want)
			want[slices.Index(want, o)] = path
			if diff := cmp.Diff(want, plan.Args); diff != "" {
				t.Errorf("args (-want +got):\n%s", diff)
			}
			if got, _ := envGet(plan.Env, "COMPOSE_FILE"); got != "ignored.yaml" {
				t.Errorf("COMPOSE_FILE changed to %q", got)
			}
		})
	}
}

func TestNoMatchingServiceLeavesPlanUnchanged(t *testing.T) {
	tmp := useTempDir(t)
	dir := project(t, map[string]string{"compose.yaml": simpleCompose})
	argv := []string{"docker", "compose", "up"}
	environ := []string{"A=" + canary}
	plan, err := compose.Prepare(context.Background(), compose.Input{
		Argv: argv, Environ: environ, Parent: environ,
		Keys: map[string][]string{filepath.Join(dir, "other.env"): {"A"}}, Cwd: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(argv, plan.Args); diff != "" {
		t.Errorf("args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(environ, plan.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if plan.Override != "" {
		t.Errorf("Plan.Override = %q, want empty", plan.Override)
	}
	if err := plan.Cleanup(); err != nil {
		t.Errorf("Cleanup: %v", err)
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("files written: %v", entries)
	}
}

func TestNoComposeFile(t *testing.T) {
	dir := t.TempDir()
	argv := []string{"docker", "compose", "up"}
	plan, path := prepare(t, argv, nil, map[string][]string{filepath.Join(dir, ".env"): {"A"}}, dir)
	if path != "" {
		t.Errorf("override written: %s", path)
	}
	if diff := cmp.Diff(argv, plan.Args); diff != "" {
		t.Errorf("args (-want +got):\n%s", diff)
	}
}

func TestMissingFlagFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	_, err := compose.Prepare(context.Background(), compose.Input{
		Argv: []string{"docker", "compose", "-f", "nope.yaml", "up"},
		Keys: map[string][]string{filepath.Join(dir, ".env"): {"A"}}, Cwd: dir,
	})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestPrepareStopsOnCanceledContext(t *testing.T) {
	useTempDir(t)
	dir := project(t, map[string]string{"compose.yaml": simpleCompose})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := compose.Prepare(ctx, compose.Input{
		Argv: []string{"docker", "compose", "up"},
		Keys: map[string][]string{filepath.Join(dir, ".env"): {"A"}}, Cwd: dir,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Prepare error = %v, want context.Canceled", err)
	}
}

func TestComposeSettingsComeFromParentOnly(t *testing.T) {
	// An env file can set COMPOSE_FILE and COMPOSE_PATH_SEPARATOR, even to
	// text built from a secret. secretref decides from the parent
	// environment only, and pins both for compose when it sets COMPOSE_FILE.
	dir := project(t, map[string]string{"compose.yaml": simpleCompose})
	sep := string(os.PathListSeparator)
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	plan, path := prepareInput(t, compose.Input{
		Argv:     []string{"docker", "compose", "up"},
		Environ:  []string{"PATH=/bin", "COMPOSE_FILE=x" + canary, "COMPOSE_PATH_SEPARATOR=,"},
		Parent:   []string{"PATH=/bin"},
		Keys:     keys,
		FileKeys: []string{"A", "COMPOSE_FILE", "COMPOSE_PATH_SEPARATOR"},
		Cwd:      dir,
	})
	if path == "" {
		t.Fatal("no override written")
	}
	got, _ := envGet(plan.Env, "COMPOSE_FILE")
	if want := filepath.Join(dir, "compose.yaml") + sep + path; got != want {
		t.Errorf("COMPOSE_FILE = %q, want %q", got, want)
	}
	if got, _ := envGet(plan.Env, "COMPOSE_PATH_SEPARATOR"); got != sep {
		t.Errorf("COMPOSE_PATH_SEPARATOR = %q, want %q", got, sep)
	}
	if !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, "COMPOSE_FILE") }) {
		t.Errorf("Warnings = %q, want one naming COMPOSE_FILE", plan.Warnings)
	}
	for _, w := range plan.Warnings {
		if strings.Contains(w, canary) {
			t.Errorf("warning leaks an env-file value: %q", w)
		}
	}
}

func TestComposeFileErrorsNameTheVariable(t *testing.T) {
	// Paths from COMPOSE_FILE never appear in an error; the error names the
	// variable instead.
	dir := project(t, map[string]string{canary + "-bad.yaml": "services: [\n"})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	for name, file := range map[string]string{"missing": canary + ".yaml", "invalid": canary + "-bad.yaml"} {
		t.Run(name, func(t *testing.T) {
			useTempDir(t)
			env := []string{"COMPOSE_FILE=" + file}
			_, err := compose.Prepare(context.Background(), compose.Input{
				Argv: []string{"docker", "compose", "up"}, Environ: env, Parent: env, Keys: keys, Cwd: dir,
			})
			if err == nil {
				t.Fatal("want error")
			}
			if msg := err.Error(); strings.Contains(msg, canary) || !strings.Contains(msg, "COMPOSE_FILE") {
				t.Errorf("error = %q, want it to name COMPOSE_FILE and not the path", msg)
			}
		})
	}
}

func readServices(t *testing.T, path string) map[string][]string {
	t.Helper()
	_, o := readOverride(t, path)
	got := map[string][]string{}
	for name, svc := range o.Services {
		for k := range svc.Environment {
			got[name] = append(got[name], k)
		}
		slices.Sort(got[name])
	}
	return got
}

func TestServiceEnvironmentKeepsItsKeys(t *testing.T) {
	// A key the service sets in environment:, in any compose file and in
	// either form, stays as the service sets it.
	files := map[string]string{}
	for _, name := range []string{"environment_base.yaml", "environment_extra.yaml"} {
		data, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // G304: a fixed testdata file.
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	dir := project(t, files)
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A", "B", "C", "D", "E"}}
	argv := []string{"docker", "compose", "-f", "environment_base.yaml", "-f", "environment_extra.yaml", "up"}
	_, path := prepare(t, argv, nil, keys, dir)
	want := map[string][]string{
		"map_form":       {"C", "E"},
		"list_form":      {"B", "D", "E"},
		"sets_all":       {"E"},
		"no_environment": {"A", "B", "C", "D", "E"},
	}
	if diff := cmp.Diff(want, readServices(t, path)); diff != "" {
		t.Errorf("override services (-want +got):\n%s", diff)
	}

	// When the services set every key themselves, nothing is written.
	dir = project(t, map[string]string{
		"compose.yaml": "services:\n  web:\n    env_file: .env\n    environment: {A: x}\n",
	})
	keys = map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	plan, path := prepare(t, []string{"docker", "compose", "up"}, []string{"A=1"}, keys, dir)
	if path != "" || plan.Override != "" {
		t.Errorf("override written: %s", path)
	}
	if diff := cmp.Diff([]string{"A=1"}, plan.Env); diff != "" {
		t.Errorf("env changed (-want +got):\n%s", diff)
	}
}

func TestKeysComposeCannotSubstituteAreSkipped(t *testing.T) {
	// Compose substitutes only ${NAME} with NAME of letters, digits, and _,
	// not starting with a digit. Other keys get one warning each.
	dir := project(t, map[string]string{
		"compose.yaml": "services:\n  web:\n    env_file: .env\n  worker:\n    env_file: .env\n",
	})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"a.b", "OK", "x-y", "_ok2", "9x"}}
	plan, path := prepare(t, []string{"docker", "compose", "up"}, nil, keys, dir)
	want := map[string][]string{"web": {"OK", "_ok2"}, "worker": {"OK", "_ok2"}}
	if diff := cmp.Diff(want, readServices(t, path)); diff != "" {
		t.Errorf("override services (-want +got):\n%s", diff)
	}
	var named []string
	for _, w := range plan.Warnings {
		named = append(named, strings.Fields(w)[0])
	}
	if diff := cmp.Diff([]string{"9x", "a.b", "x-y"}, named); diff != "" {
		t.Errorf("warned keys (-want +got):\n%s\nWarnings = %q", diff, plan.Warnings)
	}

	keys = map[string][]string{filepath.Join(dir, ".env"): {"a.b"}}
	plan, path = prepare(t, []string{"docker", "compose", "up"}, nil, keys, dir)
	if path != "" {
		t.Errorf("override written: %s", path)
	}
	if !plan.Matched || len(plan.Warnings) != 1 {
		t.Errorf("Matched = %v, Warnings = %q", plan.Matched, plan.Warnings)
	}
}

// countEnv returns how many entries of env set key.
func countEnv(env []string, key string) int {
	n := 0
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); k == key {
			n++
		}
	}
	return n
}

func warnsAbout(warnings []string, name string) bool {
	return slices.ContainsFunc(warnings, func(w string) bool { return strings.Contains(w, name) })
}

func TestFileComposeFileIsReplacedWithAWarning(t *testing.T) {
	// A plain COMPOSE_FILE in an env file never reaches the child by default,
	// but compose would read it from .env itself. secretref sets COMPOSE_FILE
	// to the files it chose and says so.
	dir := project(t, map[string]string{"compose.yaml": simpleCompose, "compose.dev.yaml": "services: {}\n"})
	sep := string(os.PathListSeparator)
	plan, path := prepareInput(t, compose.Input{
		Argv:     []string{"docker", "compose", "up"},
		Environ:  []string{"PATH=/bin", "A=" + canary},
		Parent:   []string{"PATH=/bin"},
		Keys:     map[string][]string{filepath.Join(dir, ".env"): {"A"}},
		FileKeys: []string{"COMPOSE_FILE", "A"},
		Cwd:      dir,
	})
	if path == "" {
		t.Fatal("no override written")
	}
	got, _ := envGet(plan.Env, "COMPOSE_FILE")
	if want := filepath.Join(dir, "compose.yaml") + sep + path; got != want {
		t.Errorf("COMPOSE_FILE = %q, want %q", got, want)
	}
	if !warnsAbout(plan.Warnings, "COMPOSE_FILE") {
		t.Errorf("Warnings = %q, want one naming COMPOSE_FILE", plan.Warnings)
	}
}

func TestFileComposeFileWithoutOverride(t *testing.T) {
	// With no override, a COMPOSE_FILE from an env file, here built from a
	// secret, must not reach compose either: secretref pins the files it
	// chose, or drops the variable when it found none.
	sep := string(os.PathListSeparator)
	environ := []string{"PATH=/bin", "COMPOSE_FILE=x" + canary, "COMPOSE_PATH_SEPARATOR=,"}
	fileKeys := []string{"COMPOSE_FILE", "COMPOSE_PATH_SEPARATOR"}
	keys := map[string][]string{"/elsewhere/.env": {"A"}}

	dir := project(t, map[string]string{"compose.yaml": simpleCompose})
	plan, path := prepareInput(t, compose.Input{
		Argv: []string{"docker", "compose", "up"}, Environ: environ, Parent: []string{"PATH=/bin"},
		Keys: keys, FileKeys: fileKeys, Cwd: dir,
	})
	if path != "" {
		t.Fatalf("override written: %s", path)
	}
	if got, _ := envGet(plan.Env, "COMPOSE_FILE"); got != filepath.Join(dir, "compose.yaml") {
		t.Errorf("COMPOSE_FILE = %q, want the compose file secretref found", got)
	}
	if got, _ := envGet(plan.Env, "COMPOSE_PATH_SEPARATOR"); got != sep {
		t.Errorf("COMPOSE_PATH_SEPARATOR = %q, want %q", got, sep)
	}
	if n := countEnv(plan.Env, "COMPOSE_FILE"); n != 1 {
		t.Errorf("COMPOSE_FILE set %d times", n)
	}
	if !warnsAbout(plan.Warnings, "COMPOSE_FILE") {
		t.Errorf("Warnings = %q, want one naming COMPOSE_FILE", plan.Warnings)
	}

	empty := t.TempDir()
	plan, _ = prepareInput(t, compose.Input{
		Argv: []string{"docker", "compose", "up"}, Environ: environ, Parent: []string{"PATH=/bin"},
		Keys: keys, FileKeys: fileKeys, Cwd: empty,
	})
	for _, k := range fileKeys {
		if v, ok := envGet(plan.Env, k); ok {
			t.Errorf("%s = %q, want it removed", k, v)
		}
	}
	if !warnsAbout(plan.Warnings, "COMPOSE_FILE") {
		t.Errorf("Warnings = %q, want one naming COMPOSE_FILE", plan.Warnings)
	}
}

func TestFileComposeFileWithFileFlags(t *testing.T) {
	// With -f compose ignores COMPOSE_FILE; secretref still drops the file
	// values and has nothing to warn about.
	dir := project(t, map[string]string{"one.yaml": simpleCompose})
	environ := []string{"COMPOSE_FILE=x" + canary, "COMPOSE_PATH_SEPARATOR=,"}
	plan, _ := prepareInput(t, compose.Input{
		Argv: []string{"docker", "compose", "-f", "one.yaml", "up"}, Environ: environ,
		Keys:     map[string][]string{filepath.Join(dir, ".env"): {"A"}},
		FileKeys: []string{"COMPOSE_FILE", "COMPOSE_PATH_SEPARATOR"}, Cwd: dir,
	})
	for _, k := range []string{"COMPOSE_FILE", "COMPOSE_PATH_SEPARATOR"} {
		if v, ok := envGet(plan.Env, k); ok {
			t.Errorf("%s = %q, want it removed", k, v)
		}
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("Warnings = %q, want none", plan.Warnings)
	}
}

func TestParentComposeFileWinsSilently(t *testing.T) {
	// The environment wins over the file as for any variable; no warning.
	dir := project(t, map[string]string{"compose.yaml": simpleCompose})
	env := []string{"COMPOSE_FILE=compose.yaml"}
	plan, path := prepareInput(t, compose.Input{
		Argv: []string{"docker", "compose", "up"}, Environ: env, Parent: env,
		Keys:     map[string][]string{filepath.Join(dir, ".env"): {"A"}},
		FileKeys: []string{"COMPOSE_FILE"}, Cwd: dir,
	})
	if got, _ := envGet(plan.Env, "COMPOSE_FILE"); got != "compose.yaml"+string(os.PathListSeparator)+path {
		t.Errorf("COMPOSE_FILE = %q", got)
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("Warnings = %q, want none", plan.Warnings)
	}
}

func TestComposeVariablesStayOutOfServices(t *testing.T) {
	dir := project(t, map[string]string{"compose.yaml": simpleCompose})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A", "COMPOSE_FILE", "COMPOSE_PROJECT_NAME"}}
	_, path := prepare(t, []string{"docker", "compose", "up"}, nil, keys, dir)
	if diff := cmp.Diff(map[string][]string{"web": {"A"}}, readServices(t, path)); diff != "" {
		t.Errorf("override services (-want +got):\n%s", diff)
	}
}
