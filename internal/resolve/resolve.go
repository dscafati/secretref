package resolve

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dscafati/secretref/internal/envfile"
	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

const (
	defaultConcurrency = 8
	defaultTimeout     = 30 * time.Second
)

// Options tunes a Resolver. Zero values select the defaults: Concurrency 8 and
// Timeout 30s.
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

// Resolver resolves entries through a provider registry.
type Resolver struct {
	reg       provider.Registry
	lookupEnv func(string) (string, bool)
	opts      Options
}

// New returns a Resolver that looks keys up in lookupEnv, which gives the
// process environment.
func New(reg provider.Registry, lookupEnv func(string) (string, bool), opts Options) *Resolver {
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultConcurrency
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	return &Resolver{reg: reg, lookupEnv: lookupEnv, opts: opts}
}

type kind int

const (
	kindPlain kind = iota
	kindEnv
	kindSecret
)

// slot is the working state of one key.
type slot struct {
	entry envfile.Entry
	kind  kind
	value string // plain: raw until expanded; env: the env value
	sec   secret.Secret
	state int // plain expansion: 0 todo, 1 in progress, 2 done
	fail  bool
}

// Resolve resolves entries. Later entries win over earlier ones with the same
// key; Vars keep the order of each key's first appearance.
func (r *Resolver) Resolve(ctx context.Context, entries []envfile.Entry) Result {
	var order []string
	slots := map[string]*slot{}
	for _, e := range entries {
		if s, ok := slots[e.Key]; ok {
			s.entry = e
			continue
		}
		order = append(order, e.Key)
		slots[e.Key] = &slot{entry: e}
	}

	var toResolve []*slot
	for _, key := range order {
		s := slots[key]
		s.value = s.entry.Value
		if !r.opts.IgnoreEnv {
			if v, ok := r.lookupEnv(key); ok {
				s.kind, s.value = kindEnv, v
				continue
			}
		}
		if s.entry.Ref != nil {
			s.kind = kindSecret
			toResolve = append(toResolve, s)
		}
	}

	var res Result
	var warned []Failure
	errs := r.resolveAll(ctx, toResolve)
	for i, s := range toResolve {
		err := errs[i]
		if err == nil {
			continue
		}
		if r.opts.AllowMissing {
			s.kind = kindPlain
			warned = append(warned, Failure{Key: s.entry.Key, Ref: *s.entry.Ref, Err: err})
			continue
		}
		s.fail = true
		res.Failures = append(res.Failures, Failure{Key: s.entry.Key, Ref: *s.entry.Ref, Err: err})
	}
	slices.SortFunc(res.Failures, func(a, b Failure) int { return strings.Compare(a.Key, b.Key) })
	slices.SortFunc(warned, func(a, b Failure) int { return strings.Compare(a.Key, b.Key) })
	for _, w := range warned {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %s: %s: %v; using the value from %s",
			w.Key, w.Ref.Raw, provider.Kind(w.Err), w.Err, slots[w.Key].entry.File))
	}

	lookup := lookupFunc(slots, r)
	for _, key := range order {
		s := slots[key]
		if s.fail {
			continue
		}
		v := Var{Key: key, File: s.entry.File, Ref: s.entry.Ref, FromEnv: s.kind == kindEnv}
		if s.kind == kindSecret {
			v.Value = s.sec
		} else {
			v.Value = secret.New([]byte(r.valueOf(s, lookup)))
		}
		res.Vars = append(res.Vars, v)
	}
	return res
}

// lookupFunc returns the variable lookup used during expansion: the final file
// variables first, then the process environment. Names that are not in the file
// set, and keys whose resolution failed (their value is unknown, so there is
// nothing to substitute), fall back to the process environment. With IgnoreEnv
// the file variable still wins over the environment for names it defines.
func lookupFunc(slots map[string]*slot, r *Resolver) func(string) (string, bool) {
	var self func(string) (string, bool)
	self = func(name string) (string, bool) {
		if s, ok := slots[name]; ok && !s.fail {
			return r.valueOf(s, self), true
		}
		return r.lookupEnv(name)
	}
	return self
}

// valueOf returns the final string value of s. Plain values are expanded once;
// a reference cycle expands to the empty string.
//
// SECURITY: resolved secrets and process-environment values are returned as
// they are and never expanded, so a secret containing "${X}" stays literal.
func (r *Resolver) valueOf(s *slot, lookup func(string) (string, bool)) string {
	switch s.kind {
	case kindEnv:
		return s.value
	case kindSecret:
		// SECURITY: a plain value such as DATABASE_URL=...${PW}... needs the
		// real secret text to build the composite string. The text stays inside
		// the child-bound environment: it is copied into another secret.Secret
		// and never logged or returned in an error.
		return string(s.sec.Reveal())
	}
	switch s.state {
	case 1:
		return ""
	case 2:
		return s.value
	}
	s.state = 1
	if s.entry.Quote == envfile.QuoteSingle {
		s.value = s.entry.Value
	} else {
		s.value = expand(s.entry.Value, lookup)
	}
	s.state = 2
	return s.value
}

type outcome struct {
	sec secret.Secret
	err error
}

// resolveAll resolves every slot with bounded concurrency and returns one error
// per slot, nil on success. It returns when every worker is done or the
// deadline passes, whichever comes first, so a provider that ignores its
// context cannot hang the caller. Keys still in flight then fail with a
// timeout (or "canceled" when the parent context ended).
//
// Each worker sends its outcome on its own buffered channel and never touches
// shared state; outcomes that arrive after the deadline are never read.
func (r *Resolver) resolveAll(ctx context.Context, slots []*slot) []error {
	errs := make([]error, len(slots))
	if len(slots) == 0 {
		return errs
	}
	ctx, cancel := context.WithTimeout(ctx, r.opts.Timeout)
	defer cancel()

	sem := make(chan struct{}, r.opts.Concurrency)
	outs := make([]chan outcome, len(slots))
	var wg sync.WaitGroup
	for i, s := range slots {
		out := make(chan outcome, 1)
		outs[i] = out
		rf := *s.entry.Ref
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				out <- outcome{err: ctx.Err()}
				return
			}
			sec, err := r.resolveOne(ctx, rf)
			out <- outcome{sec: sec, err: err}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}

	for i, s := range slots {
		select {
		case o := <-outs[i]:
			if o.err != nil {
				errs[i] = r.mapErr(o.err)
			} else {
				s.sec = o.sec
			}
		default:
			errs[i] = r.mapErr(ctx.Err())
		}
	}
	return errs
}

func (r *Resolver) resolveOne(ctx context.Context, rf ref.Ref) (secret.Secret, error) {
	p, err := r.reg.Lookup(rf.Scheme)
	if err != nil {
		return secret.Secret{}, err
	}
	sec, err := p.Resolve(ctx, rf)
	if err != nil {
		return secret.Secret{}, err
	}
	if rf.Field == "" {
		return sec, nil
	}
	return extractField(sec, rf.Field)
}

// mapErr gives err one of the provider kinds.
func (r *Resolver) mapErr(err error) error {
	var rerr *ref.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("timed out after %s: %w", r.opts.Timeout, provider.ErrUnavailable)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("canceled: %w", provider.ErrUnavailable)
	case errors.As(err, &rerr):
		return fmt.Errorf("%w: %w", err, provider.ErrInvalid)
	}
	return err
}

// Environ returns base plus the Vars that are not FromEnv, as KEY=value.
func (res Result) Environ(base []string) []string {
	out := slices.Clone(base)
	for _, v := range res.Vars {
		if v.FromEnv {
			continue
		}
		// SECURITY: the only place a resolved value becomes a string. The result
		// goes to the child process environment and nowhere else.
		out = append(out, v.Key+"="+string(v.Value.Reveal()))
	}
	return out
}
