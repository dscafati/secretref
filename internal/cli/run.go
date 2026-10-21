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
		_, _ = fmt.Fprintf(d.Stderr, "secretref: warning: %s; using the value from %s\n",
			failureLine(w.Failure), relPath(cwd, w.File))
	}
	if len(res.Failures) > 0 {
		for _, f := range res.Failures {
			_, _ = fmt.Fprintf(d.Stderr, "secretref: %s\n", failureLine(f))
		}
		return exitWith(exitResolve)
	}
	if o.verbose {
		logResolved(newLogger(d.Stderr), res)
	}

	env := res.Environ(d.Environ())
	if compose.Detect(argv) {
		return runCompose(ctx, d, compose.Input{
			Argv: argv, Environ: env, Parent: d.Environ(), Keys: keysByFile(res), Cwd: cwd,
		})
	}
	if runtime.GOOS == "windows" {
		code, err := d.Spawn(ctx, argv, env)
		return startResult(argv, code, err)
	}
	return startResult(argv, exitOK, d.Replace(argv, env))
}

// runCompose spawns docker compose with the override that passes the
// injected keys to the services, and removes the override afterwards.
func runCompose(ctx context.Context, d Deps, in compose.Input) (err error) {
	// Signals are held from before the override exists until it is removed,
	// so an interrupt cannot leave it behind.
	received, stop := procexec.Hold()
	defer stop()
	plan, err := compose.Prepare(ctx, in)
	if err != nil {
		return internalError(err)
	}
	defer func() {
		if cerr := plan.Cleanup(); cerr != nil && err == nil {
			err = internalError(fmt.Errorf("remove compose override: %w", cerr))
		}
	}()
	for _, w := range plan.Warnings {
		_, _ = fmt.Fprintf(d.Stderr, "secretref: compose: warning: %s\n", w)
	}
	if !plan.Matched && len(in.Keys) > 0 {
		_, _ = fmt.Fprintf(d.Stderr, "secretref: compose: no service uses %s; containers will see file values\n",
			fileList(in.Keys, in.Cwd))
	}
	if s, ok := received(); ok {
		return exitWith(procexec.SignalExitCode(s))
	}
	code, serr := d.Spawn(ctx, plan.Args, plan.Env)
	return startResult(in.Argv, code, serr)
}

// fileList names the files in keys, relative to cwd where possible, sorted.
func fileList(keys map[string][]string, cwd string) string {
	names := make([]string, 0, len(keys))
	for f := range keys {
		names = append(names, relPath(cwd, f))
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// relPath returns path relative to cwd when it lies inside cwd, and path
// unchanged otherwise.
func relPath(cwd, path string) string {
	if rel, err := filepath.Rel(cwd, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
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
		shown.File = relPath(cwd, pe.File)
		return nil, usageError("%s", shown.Error())
	case errors.Is(err, os.ErrNotExist):
		return nil, usageError("%v", err)
	case err != nil:
		return nil, internalError(err)
	}
	return entries, nil
}

// keysByFile maps each loaded file to the keys resolved from an annotation
// whose final value came from that file. Compose passes only these through
// the override, and only to services that list that file; it reads every
// other key from env_file itself.
func keysByFile(res resolve.Result) map[string][]string {
	out := map[string][]string{}
	for _, v := range res.Vars {
		if v.Ref != nil && !v.FromEnv && !v.FellBack {
			out[v.File] = append(out[v.File], v.Key)
		}
	}
	return out
}

// failureLine renders a failed key as "<key>  <ref>  <kind>[: <reason>]".
func failureLine(f resolve.Failure) string {
	return f.Key + "  " + f.Ref.Raw + "  " + describe(f.Err)
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
