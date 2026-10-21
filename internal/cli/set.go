package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

func newSetCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "set keyring://<service>/<account>",
		Short: "Store a value in the OS keyring",
		Long: `Set reads a value and stores it in the OS keyring under the reference. On a
terminal it prompts without echo; otherwise it reads stdin and drops one
trailing newline. Only keyring:// references are accepted.`,
		Example: `  secretref set keyring://myapp/DB_PASSWORD
  printf %s "$TOKEN" | secretref set keyring://myapp/API_TOKEN`,
		Args: exactlyOneRef,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, w, err := keyringWriter(d, args[0])
			if err != nil {
				return err
			}
			value, err := readValue(d, r)
			if err != nil {
				return err
			}
			// SECURITY: the plaintext buffer is cleared once the Secret holds
			// its own copy.
			s := secret.New(value)
			clear(value)
			ctx, cancel := context.WithTimeout(cmd.Context(), defaultTimeout)
			defer cancel()
			if err := w.Set(ctx, r, s); err != nil {
				return writeFailed("store", r, err)
			}
			_, _ = fmt.Fprintf(d.Stderr, "stored %s\n", r.Raw)
			return nil
		},
	}
}

func exactlyOneRef(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return usageError("want exactly one keyring://<service>/<account> reference")
	}
	return nil
}

// keyringWriter parses raw and returns the keyring Writer for it.
func keyringWriter(d Deps, raw string) (ref.Ref, provider.Writer, error) {
	r, err := ref.Parse(raw)
	if err != nil {
		return ref.Ref{}, nil, usageError("%v", err)
	}
	if r.Scheme != "keyring" {
		return ref.Ref{}, nil, usageError("only keyring:// references can be written, got %s", r.Raw)
	}
	w, ok := d.Writers[r.Scheme]
	if !ok {
		return ref.Ref{}, nil, internalError(fmt.Errorf("no writer for %s", r.Scheme))
	}
	return r, w, nil
}

// writeFailed reports a failed keyring write of r as an internal error. Set
// and delete bound the write with defaultTimeout, so an unlock prompt nobody
// answers ends as a timeout instead of a hang.
func writeFailed(op string, r ref.Ref, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return internalError(fmt.Errorf("%s %s: timed out after %s", op, r.Raw, defaultTimeout))
	}
	return internalError(fmt.Errorf("%s %s: %w", op, r.Raw, err))
}

// readValue reads the value with hidden input on a terminal, else from stdin
// with exactly one trailing newline removed.
func readValue(d Deps, r ref.Ref) ([]byte, error) {
	var value []byte
	if d.IsTerminal() {
		_, _ = fmt.Fprintf(d.Stderr, "Value for %s: ", r.Raw)
		v, err := d.ReadPassword()
		_, _ = fmt.Fprintln(d.Stderr)
		if err != nil {
			return nil, internalError(fmt.Errorf("read value: %w", err))
		}
		value = v
	} else {
		v, err := io.ReadAll(d.Stdin)
		if err != nil {
			return nil, internalError(fmt.Errorf("read value: %w", err))
		}
		if t, ok := bytes.CutSuffix(v, []byte("\r\n")); ok {
			v = t
		} else {
			v, _ = bytes.CutSuffix(v, []byte("\n"))
		}
		value = v
	}
	if len(value) == 0 {
		return nil, usageError("empty value; nothing stored")
	}
	return value, nil
}
