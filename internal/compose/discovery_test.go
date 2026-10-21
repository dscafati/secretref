package compose_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// These tests follow compose-go's cli.WithDefaultConfigPath, which docker
// compose v2 uses when neither -f nor COMPOSE_FILE names a file: look in the
// working directory, then each parent, and stop at the first directory with
// a default name; take that directory's first override file.

// composeFileList returns the COMPOSE_FILE entries the plan sets, without the
// trailing secretref override.
func composeFileList(t *testing.T, env []string, override string) []string {
	t.Helper()
	got, ok := envGet(env, "COMPOSE_FILE")
	if !ok {
		t.Fatal("COMPOSE_FILE not set")
	}
	files := strings.Split(got, string(os.PathListSeparator))
	if last := files[len(files)-1]; last != override {
		t.Fatalf("last COMPOSE_FILE entry = %q, want the override %q", last, override)
	}
	return files[:len(files)-1]
}

func mkdir(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDefaultFileInParentDirectory(t *testing.T) {
	dir := project(t, map[string]string{
		"compose.yaml":          simpleCompose,
		"compose.override.yaml": "services: {}\n",
	})
	cwd := mkdir(t, filepath.Join(dir, "sub", "deeper"))
	// env_file paths resolve against the compose file's directory, not cwd.
	keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
	plan, path := prepare(t, []string{"docker", "compose", "up"}, nil, keys, cwd)
	if path == "" {
		t.Fatal("no override written: the compose file in a parent directory was not found")
	}
	want := []string{filepath.Join(dir, "compose.yaml"), filepath.Join(dir, "compose.override.yaml")}
	if diff := cmp.Diff(want, composeFileList(t, plan.Env, path)); diff != "" {
		t.Errorf("COMPOSE_FILE (-want +got):\n%s", diff)
	}
}

func TestNearestDirectoryWins(t *testing.T) {
	dir := project(t, map[string]string{
		"compose.yaml":              simpleCompose,
		"compose.override.yaml":     "services: {}\n",
		"sub/docker-compose.yml":    simpleCompose,
		"sub/other/compose.yaml":    "services: {}\n",
		"sub/compose.override.yaml": "services: {}\n",
	})
	sub := filepath.Join(dir, "sub")
	cwd := mkdir(t, filepath.Join(sub, "deeper"))
	keys := map[string][]string{filepath.Join(sub, ".env"): {"A"}}
	plan, path := prepare(t, []string{"docker", "compose", "up"}, nil, keys, cwd)
	if path == "" {
		t.Fatal("no override written")
	}
	want := []string{filepath.Join(sub, "docker-compose.yml"), filepath.Join(sub, "compose.override.yaml")}
	if diff := cmp.Diff(want, composeFileList(t, plan.Env, path)); diff != "" {
		t.Errorf("COMPOSE_FILE (-want +got):\n%s", diff)
	}
}

func TestProjectDirectoryWalksUp(t *testing.T) {
	// --project-directory replaces cwd as the start of the search, and stays
	// the directory env_file paths resolve against.
	dir := project(t, map[string]string{"compose.yaml": simpleCompose})
	projDir := mkdir(t, filepath.Join(dir, "app"))
	cwd := t.TempDir()
	keys := map[string][]string{filepath.Join(projDir, ".env"): {"A"}}
	for _, flag := range [][]string{{"--project-directory", projDir}, {"--project-directory=" + projDir}, {"--workdir", projDir}} {
		argv := append(append([]string{"docker", "compose"}, flag...), "up")
		plan, path := prepare(t, argv, nil, keys, cwd)
		if path == "" {
			t.Fatalf("%q: no override written", flag)
		}
		want := []string{filepath.Join(dir, "compose.yaml")}
		if diff := cmp.Diff(want, composeFileList(t, plan.Env, path)); diff != "" {
			t.Errorf("%q: COMPOSE_FILE (-want +got):\n%s", flag, diff)
		}
	}
}

func TestDefaultNameOrder(t *testing.T) {
	tests := map[string]struct {
		files []string
		want  []string
	}{
		"compose.yaml first": {
			files: []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"},
			want:  []string{"compose.yaml"},
		},
		"compose.yml next": {
			files: []string{"compose.yml", "docker-compose.yml", "docker-compose.yaml"},
			want:  []string{"compose.yml"},
		},
		"docker-compose.yml before .yaml": {
			files: []string{"docker-compose.yml", "docker-compose.yaml"},
			want:  []string{"docker-compose.yml"},
		},
		"override .yml before .yaml": {
			files: []string{"compose.yaml", "compose.override.yml", "compose.override.yaml"},
			want:  []string{"compose.yaml", "compose.override.yml"},
		},
		"any override name joins": {
			files: []string{"compose.yaml", "docker-compose.override.yaml"},
			want:  []string{"compose.yaml", "docker-compose.override.yaml"},
		},
		"compose.override before docker-compose.override": {
			files: []string{"docker-compose.yml", "docker-compose.override.yml", "compose.override.yaml"},
			want:  []string{"docker-compose.yml", "compose.override.yaml"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			contents := map[string]string{}
			for _, f := range tt.files {
				contents[f] = simpleCompose
			}
			dir := project(t, contents)
			keys := map[string][]string{filepath.Join(dir, ".env"): {"A"}}
			plan, path := prepare(t, []string{"docker", "compose", "up"}, nil, keys, dir)
			var want []string
			for _, f := range tt.want {
				want = append(want, filepath.Join(dir, f))
			}
			if diff := cmp.Diff(want, composeFileList(t, plan.Env, path)); diff != "" {
				t.Errorf("COMPOSE_FILE (-want +got):\n%s", diff)
			}
		})
	}
}
