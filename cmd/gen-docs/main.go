// Command gen-docs writes the secretref reference docs, man pages, and shell
// completions from the cobra command tree. Run it from the repository root
// through `make docs`.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/dscafati/secretref/internal/cli"
)

const (
	referenceDir   = "docs/reference"
	manDir         = "dist-docs/man"
	completionsDir = "dist-docs/completions"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gen-docs: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root := cli.NewRoot(cli.Deps{Stdout: io.Discard, Stderr: io.Discard})
	root.InitDefaultCompletionCmd()
	disableAutoGenTag(root)

	for _, dir := range []string{referenceDir, manDir, completionsDir} {
		// Regenerate from scratch so removed commands leave no stale pages.
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	if err := doc.GenMarkdownTree(root, referenceDir); err != nil {
		return fmt.Errorf("markdown: %w", err)
	}
	if err := doc.GenManTree(root, &doc.GenManHeader{Section: "1"}, manDir); err != nil {
		return fmt.Errorf("man: %w", err)
	}
	return writeCompletions(root)
}

// disableAutoGenTag drops cobra's "Auto generated ... on <date>" footer so
// the output only changes when the commands change.
func disableAutoGenTag(c *cobra.Command) {
	c.DisableAutoGenTag = true
	for _, sub := range c.Commands() {
		disableAutoGenTag(sub)
	}
}

func writeCompletions(root *cobra.Command) error {
	gens := map[string]func(io.Writer) error{
		"bash": func(w io.Writer) error { return root.GenBashCompletionV2(w, true) },
		"zsh":  root.GenZshCompletion,
		"fish": func(w io.Writer) error { return root.GenFishCompletion(w, true) },
	}
	for shell, gen := range gens {
		path := filepath.Join(completionsDir, "secretref."+shell)
		f, err := os.Create(path) //nolint:gosec // G304: fixed output path.
		if err != nil {
			return err
		}
		gerr := gen(f)
		if cerr := f.Close(); gerr == nil {
			gerr = cerr
		}
		if gerr != nil {
			return fmt.Errorf("%s completion: %w", shell, gerr)
		}
	}
	return nil
}
