package compose

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
		"--workdir":           true, // deprecated alias of --project-directory
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
		case "--project-directory", "--workdir":
			inv.projectDir = value
		}
	}
	return inv
}

// composeFile is one compose file compose will load.
type composeFile struct {
	path string // absolute
	// label names the file in errors.
	label string
}

// composeFiles returns the compose files compose will load: the -f flags,
// else COMPOSE_FILE from parent, else the default files found from
// --project-directory or cwd upwards.
//
// SECURITY: parent is the environment secretref started with. Env-file values
// never choose the files, because they can be built from a secret. Errors
// name a COMPOSE_FILE entry by position, never by its text.
func composeFiles(inv invocation, parent []string, cwd string) ([]composeFile, error) {
	if len(inv.files) > 0 {
		var files []composeFile
		for _, f := range inv.files {
			if f == "" || f == "-" {
				continue
			}
			p := absPath(f, cwd)
			if _, err := os.Stat(p); err != nil {
				return nil, fmt.Errorf("compose file: %w", err)
			}
			files = append(files, composeFile{path: p, label: "compose file " + p})
		}
		return files, nil
	}
	if v, ok := envValue(parent, composeFileVar); ok && v != "" {
		var files []composeFile
		for i, f := range strings.Split(v, pathSeparator(parent)) {
			if f == "" || f == "-" {
				continue
			}
			label := fmt.Sprintf("compose file %d in %s", i+1, composeFileVar)
			p := absPath(f, cwd)
			if _, err := os.Stat(p); err != nil {
				return nil, fmt.Errorf("%s: %w", label, pathCause(err))
			}
			files = append(files, composeFile{path: p, label: label})
		}
		return files, nil
	}
	dir := cwd
	if inv.projectDir != "" {
		dir = absPath(inv.projectDir, cwd)
	}
	var files []composeFile
	for _, p := range defaultFiles(dir) {
		files = append(files, composeFile{path: p, label: "compose file " + p})
	}
	return files, nil
}

// pathCause returns the reason inside a *fs.PathError without its path, or
// a generic error for anything else.
func pathCause(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err
	}
	return errors.New("cannot read the file")
}

// defaultFiles finds the compose files the way compose-go's
// cli.WithDefaultConfigPath does: starting at dir, it walks up to the root and
// stops at the first directory holding one of the default names. It returns
// the first default name found there plus the first override name found
// there, which need not share its stem. It returns nil when no directory has
// one.
func defaultFiles(dir string) []string {
	for {
		if base := firstExisting(dir, defaultNames()); base != "" {
			files := []string{base}
			if o := firstExisting(dir, overrideNames()); o != "" {
				files = append(files, o)
			}
			return files
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

// defaultNames are compose-go's cli.DefaultFileNames, in its order.
func defaultNames() []string {
	return []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}
}

// overrideNames are compose-go's cli.DefaultOverrideFileNames, in its order.
func overrideNames() []string {
	return []string{"compose.override.yml", "compose.override.yaml", "docker-compose.override.yml", "docker-compose.override.yaml"}
}

// firstExisting returns the path of the first name that exists in dir, or "".
// Like compose-go, it accepts anything os.Stat accepts.
func firstExisting(dir string, names []string) string {
	for _, name := range names {
		if p := filepath.Join(dir, name); exists(p) {
			return p
		}
	}
	return ""
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// projectDir is the directory compose resolves relative paths against:
// --project-directory, else the first compose file's directory.
func projectDir(inv invocation, files []composeFile, cwd string) string {
	if inv.projectDir != "" {
		return absPath(inv.projectDir, cwd)
	}
	return filepath.Dir(files[0].path)
}

// composeDoc holds the part of a compose file secretref reads.
type composeDoc struct {
	Services map[string]struct {
		EnvFile     yaml.Node `yaml:"env_file"`
		Environment yaml.Node `yaml:"environment"`
	} `yaml:"services"`
}

// matchServices returns, per service, the sorted keys the override sets: the
// keys of every file in keysByFile the service lists in env_file, minus the
// keys the service sets in environment: in any of the files. matched reports
// whether any service lists a file in keysByFile.
func matchServices(ctx context.Context, files []composeFile, dir string, keysByFile map[string][]string) (services map[string][]string, matched bool, err error) {
	wanted := map[string]map[string]bool{}
	own := map[string]map[string]bool{}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		data, err := os.ReadFile(f.path)
		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", f.label, pathCause(err))
		}
		var doc composeDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, false, fmt.Errorf("%s: %w", f.label, err)
		}
		for name, svc := range doc.Services {
			for _, k := range environmentKeys(&svc.Environment) {
				addKey(own, name, k)
			}
			for _, p := range envFilePaths(&svc.EnvFile) {
				keys, ok := keysByFile[pathKey(absPath(p, dir), foldCase())]
				matched = matched || ok
				for _, k := range keys {
					addKey(wanted, name, k)
				}
			}
		}
	}
	out := map[string][]string{}
	for name, set := range wanted {
		var keys []string
		for k := range set {
			if !own[name][k] {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			slices.Sort(keys)
			out[name] = keys
		}
	}
	return out, matched, nil
}

func addKey(sets map[string]map[string]bool, service, key string) {
	if sets[service] == nil {
		sets[service] = map[string]bool{}
	}
	sets[service][key] = true
}

// environmentKeys reads the names a service's environment: sets, in map form
// (KEY: value, or KEY with no value) or list form (KEY=value, or KEY).
func environmentKeys(n *yaml.Node) []string {
	var keys []string
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			keys = append(keys, n.Content[i].Value)
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			if item.Kind == yaml.ScalarNode {
				k, _, _ := strings.Cut(item.Value, "=")
				keys = append(keys, k)
			}
		}
	}
	return keys
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
// set in parent, else the OS path list separator.
func pathSeparator(parent []string) string {
	if v, ok := envValue(parent, pathSeparatorVar); ok && v != "" {
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
