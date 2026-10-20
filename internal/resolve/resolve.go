package resolve

import (
	"context"
	"time"

	"github.com/dscafati/secretref/internal/envfile"
	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

// Options tunes a Resolver.
type Options struct {
	Concurrency  int
	Timeout      time.Duration
	AllowMissing bool
	IgnoreEnv    bool
}

// Var is one variable for the child environment.
type Var struct {
	Key     string
	Value   secret.Secret
	File    string
	Ref     *ref.Ref
	FromEnv bool
}

// Failure is an annotated key that did not resolve.
type Failure struct {
	Key string
	Ref ref.Ref
	Err error
}

// Result is the outcome of Resolve.
type Result struct {
	Vars     []Var
	Failures []Failure
	Warnings []string
}

// Resolver resolves entries through a provider registry. Task 7 adds its
// unexported fields: the registry, the environment lookup, and the options.
type Resolver struct{}

// New returns a Resolver. Replaced in Task 7.
func New(reg provider.Registry, lookupEnv func(string) (string, bool), opts Options) *Resolver {
	panic("not implemented")
}

// Resolve resolves entries. Replaced in Task 7.
func (r *Resolver) Resolve(ctx context.Context, entries []envfile.Entry) Result {
	panic("not implemented")
}

// Environ returns base plus the Vars that are not FromEnv, as KEY=value with
// the value revealed.
func (res Result) Environ(base []string) []string {
	panic("not implemented")
}
