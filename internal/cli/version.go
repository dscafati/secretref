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
			_, err := fmt.Fprintf(d.Stdout, "secretref %s\n", build.Version)
			return err
		},
	}
}
