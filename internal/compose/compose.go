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
	// Override is the path of the override file, or "" when no service needs
	// a key and nothing was written.
	Override string
	// Matched reports whether any service lists a file in Input.Keys in
	// env_file.
	Matched bool
	// Warnings are messages for stderr. They hold variable names, never
	// values.
	Warnings []string
}

// FileSettingError reports a compose setting defined in an env file when no
// compose file was found and no -f was given. It holds names only.
type FileSettingError struct {
	Var  string // COMPOSE_FILE or COMPOSE_PATH_SEPARATOR
	File string // the env file that defines it
}

// Error implements error.
//
// SECURITY: the message names the variable and the env file, never the value.
func (e *FileSettingError) Error() string {
	return fmt.Sprintf("compose: %s is set in %s but no compose file was found; pass -f or remove %s from the env file",
		e.Var, e.File, e.Var)
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
	// FileKeys maps every key defined in the loaded env files to the last
	// file that defines it. Prepare uses names only, to tell whether
	// COMPOSE_FILE or COMPOSE_PATH_SEPARATOR came from a file.
	FileKeys map[string]string
	// Cwd is the working directory.
	Cwd string
}

// Prepare builds the Plan for in.Argv. Services whose env_file names a file
// in in.Keys get an override that sets each of its keys to ${KEY}, so compose
// reads the value from its own environment.
//
// When no service matches, no file is written and the Plan carries in.Argv
// and in.Environ, apart from the COMPOSE_FILE handling below.
//
// SECURITY: COMPOSE_FILE and COMPOSE_PATH_SEPARATOR from an env file can be
// built from a secret, and compose prints a COMPOSE_FILE it cannot open.
// Prepare never reads their file values, removes them from compose's
// environment, and, without -f, sets COMPOSE_FILE to the files secretref
// chose so compose does not read the value from .env itself.
func Prepare(ctx context.Context, in Input) (Plan, error) {
	argv, parent, cwd := in.Argv, in.Parent, in.Cwd
	inv := parseArgs(argv)
	environ := in.Environ
	fromFile := map[string]string{} // setting to the env file that defines it
	for _, k := range []string{composeFileVar, pathSeparatorVar} {
		if _, inParent := envValue(parent, k); inParent {
			continue
		}
		for name, file := range in.FileKeys {
			if sameKey(name, k) {
				fromFile[k] = file
				environ = unsetEnv(environ, k)
			}
		}
	}
	// pin sets COMPOSE_FILE to list, split the way secretref split it.
	sep := pathSeparator(parent)
	pin := func(env []string, list string) []string {
		return setEnv(setEnv(env, composeFileVar, list), pathSeparatorVar, sep)
	}
	var warnings []string
	_, fileComposeFile := fromFile[composeFileVar]
	pinFiles := fileComposeFile && inv.lastFileEnd == 0
	if pinFiles {
		warnings = append(warnings, "ignoring "+composeFileVar+" from an env file; set it in the environment or pass -f")
	}

	files, err := composeFiles(inv, parent, cwd)
	if err != nil {
		return Plan{}, err
	}
	chosen := func() string {
		if v, ok := envValue(parent, composeFileVar); ok && v != "" {
			return v
		}
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.path
		}
		return strings.Join(paths, sep)
	}
	unchanged := Plan{Args: argv, Env: environ, Cleanup: func() error { return nil }, Warnings: warnings}
	if pinFiles && len(files) > 0 {
		unchanged.Env = pin(environ, chosen())
	}
	if len(files) == 0 {
		// Nothing to pin, and compose would read the setting from .env
		// itself, expanded against an environment that holds secrets.
		for _, k := range []string{composeFileVar, pathSeparatorVar} {
			if file, ok := fromFile[k]; ok && inv.lastFileEnd == 0 {
				return Plan{}, &FileSettingError{Var: k, File: file}
			}
		}
		return unchanged, nil
	}
	services, matched, err := matchServices(ctx, files, projectDir(inv, files, cwd), absKeys(in.Keys, cwd))
	if err != nil {
		return Plan{}, err
	}
	unchanged.Matched = matched
	services, skipped := substitutable(services)
	for _, k := range skipped {
		warnings = append(warnings, k+" is not a name compose can substitute; containers will see the value from env_file")
	}
	unchanged.Warnings = warnings
	if len(services) == 0 {
		return unchanged, nil
	}

	path, err := writeOverride(services)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Args: argv, Env: environ, Cleanup: func() error { return os.Remove(path) }, Override: path, Matched: true, Warnings: warnings}
	if inv.lastFileEnd > 0 {
		args := slices.Clone(argv[:inv.lastFileEnd])
		args = append(args, "-f", path)
		args = append(args, argv[inv.lastFileEnd:]...)
		plan.Args = args
		return plan, nil
	}
	plan.Env = pin(environ, chosen()+sep+path)
	return plan, nil
}

// substitutable splits services into the keys compose can substitute as
// ${KEY} and the sorted, distinct keys it cannot. Services left with no key
// are dropped.
func substitutable(services map[string][]string) (map[string][]string, []string) {
	out := map[string][]string{}
	var skipped []string
	for name, keys := range services {
		var ok []string
		for _, k := range keys {
			if strings.HasPrefix(k, "COMPOSE_") {
				// Compose's own settings, not service variables.
				continue
			}
			if isComposeName(k) {
				ok = append(ok, k)
			} else if !slices.Contains(skipped, k) {
				skipped = append(skipped, k)
			}
		}
		if len(ok) > 0 {
			out[name] = ok
		}
	}
	slices.Sort(skipped)
	return out, skipped
}

// isComposeName reports whether compose interpolates ${name}: a letter or _,
// then letters, digits, or _.
func isComposeName(name string) bool {
	for i, c := range name {
		letter := c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return name != ""
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
