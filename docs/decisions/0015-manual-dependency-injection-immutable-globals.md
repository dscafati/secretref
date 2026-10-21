# 0015. Manual dependency injection; immutable package-level values allowed

Status: Accepted

Date: 2026-10-21

## Context

[0006](0006-manual-dependency-injection.md) chose manual dependency injection and banned package globals except `build.Version`. The ban exists to keep out mutable shared state and hidden dependencies. Those make tests order-dependent, stop them from running in parallel, and hide wiring.

`build.Version` is allowed because the linker's `-X` flag can only set package-level strings.

The code also needs two other kinds of package-level variable, and neither holds state:

- Sentinel errors such as `var ErrNotFound = errors.New("not found")`. `errors.Is` compares identity, so a sentinel must be one shared value. Go cannot declare an error as a constant.
- Compiled regular expressions such as `var re = regexp.MustCompile(...)`. A `*regexp.Regexp` is immutable and safe for concurrent use. Compiling once at package level avoids recompiling on every call.

The `gochecknoglobals` linter already exempts both. The written rule in 0006 forbids them, so the rule and the lint disagree.

## Decision

Constructors receive their dependencies. `cmd/secretref/main.go` wires everything. There is no DI framework and no `init()` registration.

Providers implement small interfaces (`Provider`, `Writer`) defined next to their consumers. Tests pass `provider/fake`.

There is no mutable package-level state. The only package-level variables are:

- `build.Version`, set by the linker,
- sentinel errors created with `errors.New`,
- regular expressions compiled with `regexp.MustCompile`.

Nothing reassigns them after initialization.

This record supersedes 0006.

## Consequences

- Wiring is visible in one file.
- Tests build only the pieces they need, and can run in parallel.
- Constructors can grow long argument lists. We group related arguments into small structs when that happens.
- The written rule matches what `gochecknoglobals` enforces.
- Reviewers reject any other package-level variable, and any code that reassigns an allowed one.
- Compile-time interface checks such as `var _ provider.Provider = (*Provider)(nil)` declare no usable variable and are not affected.
