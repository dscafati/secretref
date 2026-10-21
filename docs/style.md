# Code style

Tooling enforces formatting and most lint rules (`make check`). This page covers what
tools cannot.

## Structure

- Dependency injection is manual. Constructors receive their dependencies; wiring
  happens only in `cmd/secretref/main.go`. No framework.
- No mutable package-level state. The only package-level variables are
  `build.Version`, sentinel errors from `errors.New`, and compiled regexps from
  `regexp.MustCompile`, and nothing reassigns them. See
  [decision 0015](decisions/0015-manual-dependency-injection-immutable-globals.md).
- Define small interfaces in the package that consumes them, not the one that
  implements them.
- Packages are small and named for what they own. No `utils`, `common`, or `helpers`.
- One responsibility per file.
- Abstract on the third use, not the second.

## Comments

- Every exported name has a doc comment that starts with the name.
- Every package has a `doc.go`.
- Comments explain why, not what.
- Mark security-relevant code with a `// SECURITY:` comment saying what must hold.
- No commented-out code.
- A `TODO` carries an issue number: `// TODO(#123): ...`.

## Secrets

- Resolved values use `secret.Secret`. It prints `[redacted]` under every formatting verb.
  Call `Reveal()` only where the value is handed to the child process.
- Errors name the key and the ref, never the value.
- Never log a value, not even at debug level.

## Errors, I/O, and logging

- `context.Context` is the first parameter of anything that does I/O.
- Wrap with `%w`; provider errors wrap the sentinel kinds `NotFound`, `Denied`,
  `Unavailable`, `Invalid`.
- No `panic` outside `main`.
- Log with `log/slog` to stderr. Stdout belongs to the child process.

## Tests

- Standard `testing` plus `go-cmp`. No testify.
- Table-driven where cases share a shape.
- Prefer hand-written fakes over mocks.
- Golden files live in `testdata/`; regenerate with `-update` and review the diff.
- Every bug fix adds a test that fails first.
- Acceptance and integration tests resolve a known canary value and fail if it
  appears anywhere except the child's environment.
