package compose

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const composeFileVar = "COMPOSE_FILE"

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
}

// Prepare builds the Plan for argv. keysByFile maps each env file secretref
// loaded to the keys it injects; environ is the child environment. Services
// whose env_file names one of those files get an override that sets each key
// to ${KEY}, so compose reads the value from its own environment.
//
// When no service matches, the Plan carries argv and environ unchanged and no
// file is written.
func Prepare(argv, environ []string, keysByFile map[string][]string, cwd string) (Plan, error) {
	unchanged := Plan{Args: argv, Env: environ, Cleanup: func() error { return nil }}
	inv := parseArgs(argv)
	files, err := composeFiles(inv, environ, cwd)
	if err != nil {
		return Plan{}, err
	}
	if len(files) == 0 {
		return unchanged, nil
	}
	services, err := matchServices(files, projectDir(inv, files, cwd), absKeys(keysByFile, cwd))
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
	plan := Plan{Args: argv, Env: environ, Cleanup: func() error { return os.Remove(path) }, Override: path}
	if inv.lastFileEnd > 0 {
		args := slices.Clone(argv[:inv.lastFileEnd])
		args = append(args, "-f", path)
		args = append(args, argv[inv.lastFileEnd:]...)
		plan.Args = args
		return plan, nil
	}
	orig, ok := envValue(environ, composeFileVar)
	if !ok || orig == "" {
		orig = strings.Join(files, pathSeparator(environ))
	}
	plan.Env = setEnv(environ, composeFileVar, orig+pathSeparator(environ)+path)
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
