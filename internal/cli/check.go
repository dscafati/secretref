package cli

import (
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/dscafati/secretref/internal/resolve"
)

type checkOptions struct {
	envFiles []string
	timeout  time.Duration
}

func newCheckCmd(d Deps) *cobra.Command {
	var o checkOptions
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Resolve every annotated variable and report the result",
		Long: `Check resolves every annotated variable, including those already set in the
environment, and prints one line per key: the key, the reference, and "ok" or
the failure. It never prints values. It exits 0 when every reference resolves
and 3 otherwise.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCheck(cmd, d, o)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&o.envFiles, "env-file", nil, "env `file` to load; repeatable; replaces .env and .env.local")
	f.DurationVar(&o.timeout, "timeout", defaultTimeout, "time limit for resolving all references")
	return cmd
}

type checkRow struct{ key, ref, status string }

func runCheck(cmd *cobra.Command, d Deps, o checkOptions) error {
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
		Concurrency: defaultConcurrency,
		Timeout:     o.timeout,
		IgnoreEnv:   true,
	}).Resolve(cmd.Context(), entries)

	var rows []checkRow
	for _, v := range res.Vars {
		if v.Ref != nil {
			rows = append(rows, checkRow{v.Key, v.Ref.Raw, "ok"})
		}
	}
	for _, f := range res.Failures {
		rows = append(rows, checkRow{f.Key, f.Ref.Raw, describe(f.Err)})
	}
	slices.SortFunc(rows, func(a, b checkRow) int { return strings.Compare(a.key, b.key) })

	tw := tabwriter.NewWriter(d.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tREF\tSTATUS")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", r.key, r.ref, r.status)
	}
	if err := tw.Flush(); err != nil {
		return internalError(err)
	}
	if len(res.Failures) > 0 {
		return exitWith(exitResolve)
	}
	return nil
}
