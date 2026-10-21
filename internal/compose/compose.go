package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	composeFileVar   = "COMPOSE_FILE"
	pathSeparatorVar = "COMPOSE_PATH_SEPARATOR"
)

// Detect reports whether argv is a docker compose invocation: argv[0] has base
// name docker (or docker.exe) and compose is its first argument after docker's
// own global flags.
func Detect(argv []string) bool {
	return composeIndex(argv) > 0
}

// composeIndex returns the index of "compose" in a docker command line, or -1.
// An unknown docker global flag before it yields -1, because secretref cannot
// tell whether it takes a value.
func composeIndex(argv []string) int {
	if len(argv) < 2 {
		return -1
	}
	base := argv[0][strings.LastIndexAny(argv[0], `/\`)+1:]
	if !strings.EqualFold(base, "docker") && !strings.EqualFold(base, "docker.exe") {
		return -1
	}
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		if a == "compose" {
			return i
		}
		name, _, hasValue := strings.Cut(a, "=")
		takesValue, known := dockerGlobalFlags()[name]
		if !known {
			return -1
		}
		if takesValue && !hasValue {
			i++
		}
	}
	return -1
}

// dockerGlobalFlags maps the docker CLI's global flags to whether they take a
// value.
func dockerGlobalFlags() map[string]bool {
	return map[string]bool{
		"--context": true, "-c": true,
		"--host": true, "-H": true,
		"--config":    true,
		"--log-level": true, "-l": true,
		"--tlscacert": true, "--tlscert": true, "--tlskey": true,
		"--debug": false, "-D": false,
		"--tls": false, "--tlsverify": false,
	}
}

// Plan is the command line and environment to run in compose mode.
type Plan struct {
	Args    []string
	Env     []string
	Cleanup func() error
	// Override is the path of the override file, or "" when no service uses
	// a loaded env file and nothing was written.
	Override string
	// Warnings are messages for stderr. They hold variable names, never
	// values.
	Warnings []string
}

// Input is what Prepare reads.
type Input struct {
	// Argv is the command line, starting with docker.
	Argv []string
	// Environ is the environment the child gets.
	Environ []string
	// Parent is the environment secretref itself started with. COMPOSE_FILE
	// and COMPOSE_PATH_SEPARATOR are read from here only, never from Environ,
	// which holds env-file values that can be built from a secret.
	Parent []string
	// Keys maps each env file secretref loaded to the keys it passes to the
	// services that list that file in env_file.
	Keys map[string][]string
	// Cwd is the working directory.
	Cwd string
}

// Prepare builds the Plan for in.Argv. Services whose env_file names a file
// in in.Keys get an override that sets each of its keys to ${KEY}, so compose
// reads the value from its own environment.
//
// When no service matches, the Plan carries in.Argv and in.Environ unchanged
// and no file is written.
func Prepare(ctx context.Context, in Input) (Plan, error) {
	argv, environ, parent, cwd := in.Argv, in.Environ, in.Parent, in.Cwd
	inv := parseArgs(argv)
	var warnings []string
	if _, inParent := envValue(parent, composeFileVar); !inParent && inv.lastFileEnd == 0 {
		if _, inChild := envValue(environ, composeFileVar); inChild {
			warnings = append(warnings, "ignoring "+composeFileVar+" from an env file; set it in the environment or pass -f")
		}
	}
	unchanged := Plan{Args: argv, Env: environ, Cleanup: func() error { return nil }, Warnings: warnings}
	files, err := composeFiles(inv, parent, cwd)
	if err != nil {
		return Plan{}, err
	}
	if len(files) == 0 {
		return unchanged, nil
	}
	services, err := matchServices(ctx, files, projectDir(inv, files, cwd), absKeys(in.Keys, cwd))
	if err != nil {
		return Plan{}, err
	}
	if len(services) == 0 {
		return unchanged, nil
	}

	path, err := writeOverride(services)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Args: argv, Env: environ, Cleanup: func() error { return os.Remove(path) }, Override: path, Warnings: warnings}
	if inv.lastFileEnd > 0 {
		args := slices.Clone(argv[:inv.lastFileEnd])
		args = append(args, "-f", path)
		args = append(args, argv[inv.lastFileEnd:]...)
		plan.Args = args
		return plan, nil
	}
	// Compose reads both variables from its environment, and from the
	// project's .env when they are unset there. Setting both makes compose
	// split COMPOSE_FILE the way secretref built it.
	sep := pathSeparator(parent)
	orig, ok := envValue(parent, composeFileVar)
	if !ok || orig == "" {
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.path
		}
		orig = strings.Join(paths, sep)
	}
	plan.Env = setEnv(setEnv(environ, composeFileVar, orig+sep+path), pathSeparatorVar, sep)
	return plan, nil
}

// override is the YAML document written for compose.
type override struct {
	Services map[string]overrideService `yaml:"services"`
}

type overrideService struct {
	Environment map[string]string `yaml:"environment"`
}

// writeOverride writes the override for services (service name to keys) to a
// new temporary file and returns its path.
//
// SECURITY: the file holds names only. Each value is the literal text ${KEY},
// which compose interpolates from the environment secretref gives it.
func writeOverride(services map[string][]string) (string, error) {
	doc := override{Services: map[string]overrideService{}}
	for name, keys := range services {
		env := make(map[string]string, len(keys))
		for _, k := range keys {
			env[k] = "${" + k + "}"
		}
		doc.Services[name] = overrideService{Environment: env}
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("compose override: %w", err)
	}
	// os.CreateTemp creates the file with mode 0600.
	f, err := os.CreateTemp("", "secretref-compose-override-*.yaml")
	if err != nil {
		return "", fmt.Errorf("compose override: %w", err)
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("compose override: %w", err)
	}
	return f.Name(), nil
}

// absKeys returns keysByFile keyed by pathKey of each absolute, clean path.
func absKeys(keysByFile map[string][]string, cwd string) map[string][]string {
	out := make(map[string][]string, len(keysByFile))
	for p, keys := range keysByFile {
		k := pathKey(absPath(p, cwd), foldCase())
		out[k] = append(out[k], keys...)
	}
	return out
}

// pathKey returns the form of p used to compare paths. Windows paths compare
// case-insensitively.
func pathKey(p string, fold bool) string {
	if fold {
		return strings.ToLower(p)
	}
	return p
}

func foldCase() bool { return runtime.GOOS == "windows" }

func absPath(p, base string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}
