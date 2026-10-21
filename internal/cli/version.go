package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dscafati/secretref/internal/build"
)

func newVersionCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the secretref version",
		Args:  noArgs,
		RunE: func(*cobra.Command, []string) error {
			return printVersion(d)
		},
	}
}

// printVersion writes the line that both the version command and the root
// --version flag print.
func printVersion(d Deps) error {
	_, err := fmt.Fprintf(d.Stdout, "secretref %s\n", build.Resolve(d.ReadBuildInfo))
	return err
}
