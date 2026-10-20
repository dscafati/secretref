// Command secretref starts a command with secrets resolved from annotated
// dotenv files.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/term"

	"github.com/dscafati/secretref/internal/cli"
	"github.com/dscafati/secretref/internal/procexec"
	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/provider/awssm"
	"github.com/dscafati/secretref/internal/provider/gcpsm"
	"github.com/dscafati/secretref/internal/provider/keyring"
)

func main() {
	os.Exit(run())
}

func run() int {
	kr := keyring.New()
	reg := provider.Registry{
		"aws-sm":  awssm.New(),
		"gcp-sm":  gcpsm.New(gcpOptions()),
		"keyring": kr,
	}
	writers := map[string]provider.Writer{"keyring": kr}
	if err := installTestProviders(reg, writers); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "secretref: %v\n", err)
		return 1
	}

	stdin := int(os.Stdin.Fd())
	return cli.Execute(context.Background(), os.Args[1:], cli.Deps{
		Registry:     reg,
		Writers:      writers,
		LookupEnv:    os.LookupEnv,
		Environ:      os.Environ,
		Getwd:        os.Getwd,
		Stdin:        os.Stdin,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		IsTerminal:   func() bool { return term.IsTerminal(stdin) },
		ReadPassword: func() ([]byte, error) { return term.ReadPassword(stdin) },
		Replace:      procexec.Replace,
		Spawn:        procexec.Spawn,
	})
}

// gcpOptions points the GCP provider at SECRETREF_GCPSM_ENDPOINT when set.
// The variable exists for tests against a fake server.
func gcpOptions() gcpsm.Options {
	if ep := os.Getenv("SECRETREF_GCPSM_ENDPOINT"); ep != "" {
		return gcpsm.Options{BaseURL: ep, HTTPClient: http.DefaultClient}
	}
	return gcpsm.Options{}
}
