package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dscafati/secretref/internal/compose"
	"github.com/dscafati/secretref/internal/envfile"
	"github.com/dscafati/secretref/internal/procexec"
	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/resolve"
)

const (
	defaultConcurrency = 8
	defaultTimeout     = 30 * time.Second
	runUsage           = "run needs -- followed by a command, as in: secretref run -- npm start"
)

type runOptions struct {
	envFiles     []string
	allowMissing bool
	timeout      time.Duration
	verbose      bool
}

func newRunCmd(d Deps) *cobra.Command {
	var o runOptions
	cmd := &cobra.Command{
		Use:   "run [flags] -- <command> [args...]",
		Short: "Resolve annotated variables and start a command",
		Long: `Run loads .env then .env.local (or the --env-file list), resolves annotated
variables that are not already set in the environment, and starts the command
with every file variable in its environment. Variables already set win.

On Unix secretref replaces itself with the command. On Windows, and for
"docker compose", it starts the command as a child, forwards interrupts, and
exits with the child's exit code.`,
		Example: `  secretref run -- npm start
  secretref run --env-file .env.staging -- ./server
  secretref run -- docker compose up`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd.Context(), d, o, cmd.ArgsLenAtDash(), args)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&o.envFiles, "env-file", nil, "env `file` to load; repeatable; replaces .env and .env.local")
	f.BoolVar(&o.allowMissing, "allow-missing", false, "use the file's value for references that fail, with a warning")
	f.DurationVar(&o.timeout, "timeout", defaultTimeout, "time limit for resolving all references")
	f.BoolVar(&o.verbose, "verbose", false, "log which key resolved from which reference (never values)")
	return cmd
}

func runRun(ctx context.Context, d Deps, o runOptions, dash int, argv []string) error {
	if dash != 0 || len(argv) == 0 {
		return usageError(runUsage)
	}
	if o.timeout <= 0 {
		return usageError("--timeout must be positive")
	}
	cwd, err := d.Getwd()
	if err != nil {
		return internalError(err)
	}
	entries, err := loadEntries(cwd, o.envFiles)
	if err != nil {
		return err
	}

	res := resolve.New(d.Registry, d.LookupEnv, resolve.Options{
		Concurrency:  defaultConcurrency,
		Timeout:      o.timeout,
		AllowMissing: o.allowMissing,
	}).Resolve(ctx, entries)
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintf(d.Stderr, "secretref: warning: %s\n", w)
	}
	if len(res.Failures) > 0 {
		for _, f := range res.Failures {
			_, _ = fmt.Fprintf(d.Stderr, "secretref: %s  %s  %s\n", f.Key, f.Ref.Raw, describe(f.Err))
		}
		return exitWith(exitResolve)
	}
	if o.verbose {
		logResolved(newLogger(d.Stderr), res)
	}

	env := res.Environ(d.Environ())
	if compose.Detect(argv) {
		return runCompose(ctx, d, argv, env, keysByFile(entries, res), cwd)
	}
	if runtime.GOOS == "windows" {
		code, err := d.Spawn(ctx, argv, env)
		return startResult(argv, code, err)
	}
	return startResult(argv, exitOK, d.Replace(argv, env))
}

// runCompose spawns docker compose with the override that passes the
// injected keys to the services, and removes the override afterwards.
func runCompose(ctx context.Context, d Deps, argv, env []string, keys map[string][]string, cwd string) (err error) {
	// Signals are held from before the override exists until it is removed,
	// so an interrupt cannot leave it behind.
	received, stop := procexec.Hold()
	defer stop()
	plan, err := compose.Prepare(argv, env, keys, cwd)
	if err != nil {
		return internalError(err)
	}
	defer func() {
		if cerr := plan.Cleanup(); cerr != nil && err == nil {
			err = internalError(fmt.Errorf("remove compose override: %w", cerr))
		}
	}()
	if plan.Override == "" && len(keys) > 0 {
		_, _ = fmt.Fprintf(d.Stderr, "secretref: compose: no service uses %s; containers will see file values\n",
			fileList(keys, cwd))
	}
	if s, ok := received(); ok {
		return exitWith(procexec.SignalExitCode(s))
	}
	code, serr := d.Spawn(ctx, plan.Args, plan.Env)
	return startResult(argv, code, serr)
}

// fileList names the files in keys, relative to cwd where possible, sorted.
func fileList(keys map[string][]string, cwd string) string {
	names := make([]string, 0, len(keys))
	for f := range keys {
		if rel, err := filepath.Rel(cwd, f); err == nil && !strings.HasPrefix(rel, "..") {
			f = rel
		}
		names = append(names, f)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// startResult turns the outcome of starting the command into the command's
// exit code or an error.
func startResult(argv []string, code int, err error) error {
	switch {
	case errors.Is(err, procexec.ErrCommandNotFound):
		return &exitError{code: exitNotFound, msg: "command not found: " + argv[0]}
	case err != nil:
		return internalError(err)
	case code != exitOK:
		return exitWith(code)
	}
	return nil
}

// loadEntries reads the --env-file list, or .env and .env.local in cwd.
// Paths become absolute so compose can match them against env_file.
func loadEntries(cwd string, flagged []string) ([]envfile.Entry, error) {
	var paths []string
	for _, p := range flagged {
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		paths = append(paths, p)
	}
	if len(flagged) == 0 {
		paths = envfile.DefaultFiles(cwd)
		if len(paths) == 0 {
			return nil, usageError("no .env or .env.local found in %s", cwd)
		}
	}
	entries, err := envfile.Load(paths)
	var pe *envfile.ParseError
	switch {
	case errors.As(err, &pe):
		shown := *pe
		if rel, rerr := filepath.Rel(cwd, pe.File); rerr == nil && !strings.HasPrefix(rel, "..") {
			shown.File = rel
		}
		return nil, usageError("%s", shown.Error())
	case errors.Is(err, os.ErrNotExist):
		return nil, usageError("%v", err)
	case err != nil:
		return nil, internalError(err)
	}
	return entries, nil
}

// keysByFile maps each loaded file to the keys it defines that the child
// receives from secretref, so compose services listing that file get them.
func keysByFile(entries []envfile.Entry, res resolve.Result) map[string][]string {
	injected := map[string]bool{}
	for _, v := range res.Vars {
		if !v.FromEnv {
			injected[v.Key] = true
		}
	}
	out := map[string][]string{}
	seen := map[string]bool{}
	for _, e := range entries {
		id := e.File + "\x00" + e.Key
		if injected[e.Key] && !seen[id] {
			seen[id] = true
			out[e.File] = append(out[e.File], e.Key)
		}
	}
	return out
}

// describe renders a resolution error as "<kind>: <reason>". Provider errors
// end with the kind they wrap ("...: not found"); that suffix is dropped so
// the kind is not printed twice.
func describe(err error) string {
	kind := provider.Kind(err)
	var rerr *ref.Error
	if kind == "error" && errors.As(err, &rerr) {
		kind = "invalid"
	}
	reason := err.Error()
	for reason != kind && strings.HasSuffix(reason, ": "+kind) {
		reason = strings.TrimSuffix(reason, ": "+kind)
	}
	if reason == kind || reason == "" {
		return kind
	}
	return kind + ": " + reason
}

func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}

// logResolved logs each key that resolved from a reference.
//
// SECURITY: logs the key and the reference only, never the value.
func logResolved(log *slog.Logger, res resolve.Result) {
	for _, v := range res.Vars {
		if v.Ref != nil && !v.FromEnv && !v.FellBack {
			log.Info(fmt.Sprintf("resolved %s from %s", v.Key, v.Ref.Raw))
		}
	}
}
