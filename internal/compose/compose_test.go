package compose_test

import (
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

// prepare runs Prepare and returns the plan and the override path ("" if
// none was written).
func prepare(t *testing.T, argv, environ []string, keys map[string][]string, cwd string) (compose.Plan, string) {
	t.Helper()
	tmp := useTempDir(t)
	plan, err := compose.Prepare(argv, environ, keys, cwd)
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
	// compose.yaml wins over docker-compose.yml; only its own override joins.
	dir := project(t, map[string]string{
		"compose.yaml":                simpleCompose,
		"docker-compose.yml":          "services: {}\n",
		"docker-compose.override.yml": "services: {}\n",
	})
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	plan, path := prepare(t, []string{"docker", "compose", "up"}, nil, keys, dir)
	got, _ := envGet(plan.Env, "COMPOSE_FILE")
	want := filepath.Join(dir, "compose.yaml") + string(os.PathListSeparator) + path
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
	plan, err := compose.Prepare(argv, environ, map[string][]string{filepath.Join(dir, "other.env"): {"A"}}, dir)
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
	_, err := compose.Prepare([]string{"docker", "compose", "-f", "nope.yaml", "up"}, nil,
		map[string][]string{filepath.Join(dir, ".env"): {"A"}}, dir)
	if err == nil {
		t.Fatal("want error")
	}
}
