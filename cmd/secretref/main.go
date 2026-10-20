// Command secretref resolves secret references found in dotenv files.
package main

import (
	"fmt"
	"os"

	"github.com/dscafati/secretref/internal/build"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is a placeholder dispatcher; the cobra wiring replaces it.
func run(args []string) int {
	if len(args) == 1 && args[0] == "version" {
		_, _ = fmt.Fprintf(os.Stdout, "secretref %s\n", build.Version)
		return 0
	}
	fmt.Fprintln(os.Stderr, "usage: secretref version")
	return 2
}
