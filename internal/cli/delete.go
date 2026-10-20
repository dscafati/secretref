package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dscafati/secretref/internal/provider"
)

func newDeleteCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "delete keyring://<service>/<account>",
		Short: "Remove a value from the OS keyring",
		Args:  exactlyOneRef,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, w, err := keyringWriter(d, args[0])
			if err != nil {
				return err
			}
			err = w.Delete(cmd.Context(), r)
			if errors.Is(err, provider.ErrNotFound) {
				return &exitError{code: exitResolve, msg: r.Raw + ": not found"}
			}
			if err != nil {
				return internalError(fmt.Errorf("delete %s: %w", r.Raw, err))
			}
			_, _ = fmt.Fprintf(d.Stderr, "deleted %s\n", r.Raw)
			return nil
		},
	}
}
