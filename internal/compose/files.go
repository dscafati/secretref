package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// invocation is what secretref needs from the compose global flags.
type invocation struct {
	files      []string // -f/--file values, in order
	projectDir string   // --project-directory
	// lastFileEnd is the index in argv just after the last -f flag and its
	// value, or 0 if there is none.
	lastFileEnd int
}

// globalValueFlags lists the compose global flags that take a value. Any
// other flag before the subcommand is treated as a boolean.
func globalValueFlags() map[string]bool {
	return map[string]bool{
		"-f": true, "--file": true,
		"-p": true, "--project-name": true,
		"--project-directory": true,
		"--profile":           true,
		"--env-file":          true,
		"--ansi":              true,
		"--progress":          true,
		"--parallel":          true,
	}
}

// parseArgs reads the compose global flags after "compose", up to the
// subcommand. Flags after the subcommand belong to it (logs -f means
// --follow).
func parseArgs(argv []string) invocation {
	var inv invocation
	valueFlags := globalValueFlags()
	for i := composeIndex(argv) + 1; i < len(argv); i++ {
		a := argv[i]
		if a == "--" || !strings.HasPrefix(a, "-") || a == "-" {
			break
		}
		name, value, hasValue := strings.Cut(a, "=")
		if !hasValue && strings.HasPrefix(a, "-f") && len(a) > 2 && !strings.HasPrefix(a, "--") {
			name, value, hasValue = "-f", a[2:], true
		}
		if !valueFlags[name] {
			continue
		}
		if !hasValue {
			if i+1 >= len(argv) {
				break
			}
			i++
			value = argv[i]
		}
		switch name {
		case "-f", "--file":
			inv.files = append(inv.files, value)
			inv.lastFileEnd = i + 1
		case "--project-directory":
			inv.projectDir = value
		}
	}
	return inv
}

// composeFiles returns the compose files compose will load, as absolute paths:
// the -f flags, else COMPOSE_FILE, else the first default name found plus its
// override file.
func composeFiles(inv invocation, environ []string, cwd string) ([]string, error) {
	var named []string
	switch v, ok := envValue(environ, composeFileVar); {
	case len(inv.files) > 0:
		named = inv.files
	case ok && v != "":
		named = strings.Split(v, pathSeparator(environ))
	default:
		dir := cwd
		if inv.projectDir != "" {
			dir = absPath(inv.projectDir, cwd)
		}
		return defaultFiles(dir), nil
	}
	var files []string
	for _, f := range named {
		if f == "" || f == "-" {
			continue
		}
		p := absPath(f, cwd)
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("compose file: %w", err)
		}
		files = append(files, p)
	}
	return files, nil
}

// defaultFiles returns the first existing default compose file in dir and its
// override file, if any.
func defaultFiles(dir string) []string {
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		base := filepath.Join(dir, name)
		if !isFile(base) {
			continue
		}
		files := []string{base}
		stem, ext := strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name)
		for _, e := range []string{ext, otherExt(ext)} {
			if o := filepath.Join(dir, stem+".override"+e); isFile(o) {
				files = append(files, o)
				break
			}
		}
		return files
	}
	return nil
}

func otherExt(ext string) string {
	if ext == ".yaml" {
		return ".yml"
	}
	return ".yaml"
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// projectDir is the directory compose resolves relative paths against:
// --project-directory, else the first compose file's directory.
func projectDir(inv invocation, files []string, cwd string) string {
	if inv.projectDir != "" {
		return absPath(inv.projectDir, cwd)
	}
	return filepath.Dir(files[0])
}

// composeDoc holds the part of a compose file secretref reads.
type composeDoc struct {
	Services map[string]struct {
		EnvFile yaml.Node `yaml:"env_file"`
	} `yaml:"services"`
}

// matchServices returns, per service, the sorted keys of every loaded env
// file the service lists in env_file.
func matchServices(files []string, dir string, keysByFile map[string][]string) (map[string][]string, error) {
	sets := map[string]map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // G304: a compose file the user's command names.
		if err != nil {
			return nil, fmt.Errorf("compose file: %w", err)
		}
		var doc composeDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("compose file %s: %w", f, err)
		}
		for name, svc := range doc.Services {
			for _, p := range envFilePaths(&svc.EnvFile) {
				keys := keysByFile[pathKey(absPath(p, dir), foldCase())]
				if len(keys) == 0 {
					continue
				}
				if sets[name] == nil {
					sets[name] = map[string]bool{}
				}
				for _, k := range keys {
					sets[name][k] = true
				}
			}
		}
	}
	out := make(map[string][]string, len(sets))
	for name, set := range sets {
		keys := make([]string, 0, len(set))
		for k := range set {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		out[name] = keys
	}
	return out, nil
}

// envFilePaths reads env_file in any of its forms: a string, a list of
// strings, or a list of {path, required}.
func envFilePaths(n *yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.SequenceNode:
		var paths []string
		for _, item := range n.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				paths = append(paths, item.Value)
			case yaml.MappingNode:
				var obj struct {
					Path string `yaml:"path"`
				}
				if item.Decode(&obj) == nil && obj.Path != "" {
					paths = append(paths, obj.Path)
				}
			}
		}
		return paths
	}
	return nil
}

// pathSeparator is the COMPOSE_FILE separator: COMPOSE_PATH_SEPARATOR when
// set, else the OS path list separator.
func pathSeparator(environ []string) string {
	if v, ok := envValue(environ, "COMPOSE_PATH_SEPARATOR"); ok && v != "" {
		return v
	}
	return string(os.PathListSeparator)
}

func sameKey(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// envValue returns the last value of key in environ.
func envValue(environ []string, key string) (string, bool) {
	for i := len(environ) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(environ[i], "="); ok && sameKey(k, key) {
			return v, true
		}
	}
	return "", false
}

// setEnv returns environ without any entry for key, plus key=value.
func setEnv(environ []string, key, value string) []string {
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if k, _, _ := strings.Cut(kv, "="); !sameKey(k, key) {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+value)
}
