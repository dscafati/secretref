package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

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
		"gcp-sm":  gcpsm.New(gcpsm.Options{}),
		"keyring": kr,
	}
	writers := map[string]provider.Writer{"keyring": kr}
	if err := installTestProviders(reg, writers); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "secretref: %v\n", err)
		return 1
	}

	stdin := int(os.Stdin.Fd())
	return cli.Execute(context.Background(), os.Args[1:], cli.Deps{
		Registry:      reg,
		Writers:       writers,
		LookupEnv:     os.LookupEnv,
		Environ:       os.Environ,
		Getwd:         os.Getwd,
		Stdin:         os.Stdin,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
		IsTerminal:    func() bool { return term.IsTerminal(stdin) },
		ReadPassword:  func() ([]byte, error) { return term.ReadPassword(stdin) },
		Replace:       procexec.Replace,
		Spawn:         procexec.Spawn,
		ReadBuildInfo: debug.ReadBuildInfo,
	})
}
