package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/dscafati/secretref/internal/provider"
)

// Deps holds everything the commands touch outside the process. main wires
// the real implementations; tests pass fakes.
type Deps struct {
	Registry  provider.Registry
	Writers   map[string]provider.Writer
	LookupEnv func(string) (string, bool)
	Environ   func() []string
	Getwd     func() (string, error)
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	// IsTerminal reports whether stdin is a terminal.
	IsTerminal func() bool
	// ReadPassword reads a line from the terminal without echo.
	ReadPassword func() ([]byte, error)
	// Replace runs argv in place of the current process.
	Replace func(argv, env []string) error
	// Spawn runs argv as a child and returns its exit code.
	Spawn func(ctx context.Context, argv, env []string) (int, error)
}

// NewRoot returns the secretref command tree. It calls no Deps function, so
// gen-docs can build it with zero Deps.
func NewRoot(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "secretref",
		Short: "Start a command with secrets resolved from annotated .env files",
		Long: `secretref reads .env and .env.local, resolves the variables annotated with
a "# secretref: <ref>" comment, and starts a command with the real values in
its environment. Values are never written to disk, stdout, or logs.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		// The root runs only when no subcommand matched.
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError("unknown command %q; run 'secretref --help'", args[0])
			}
			return cmd.Help()
		},
	}
	root.SetIn(d.Stdin)
	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError("%v", err)
	})
	root.AddCommand(newRunCmd(d), newCheckCmd(d), newSetCmd(d), newDeleteCmd(d), newVersionCmd(d))
	root.InitDefaultCompletionCmd()
	tagArgErrors(root)
	return root
}

// tagArgErrors wraps every command's argument check, including cobra's own
// completion command, so that its errors are usage errors.
func tagArgErrors(c *cobra.Command) {
	if check := c.Args; check != nil {
		c.Args = func(cmd *cobra.Command, args []string) error {
			if err := check(cmd, args); err != nil {
				return asUsage(err)
			}
			return nil
		}
	}
	for _, sub := range c.Commands() {
		tagArgErrors(sub)
	}
}

// Execute runs secretref with args and returns the exit code. It never exits
// the process; main does.
func Execute(ctx context.Context, args []string, d Deps) int {
	root := NewRoot(d)
	root.SetArgs(args)
	return exitCode(root.ExecuteContext(ctx), d.Stderr)
}

// noArgs rejects positional arguments as a usage error.
func noArgs(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError("unexpected arguments: %q", args)
	}
	return nil
}
