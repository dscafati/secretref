# Review rules

Used by human and automated reviewers. Rules come from [AGENTS.md](../AGENTS.md),
[docs/style.md](../docs/style.md), and [docs/decisions/](../docs/decisions/index.md).

## Flag

- A resolved value reaching stdout, stderr, a log, an error message, or a file.
- A value type other than `secret.Secret` carrying a resolved value.
- Code that overrides or resolves a variable already present in the process environment.
- `set` or `delete` accepting a scheme other than `keyring://`.
- A new provider, flag, or env var not covered by an ADR.
- Anything that needs CGO, or breaks a linux/darwin/windows x amd64/arm64 build.
- Package-level mutable state; wiring outside `cmd/secretref/main.go`.
- A bug fix without a test that fails without the fix.
- A decision reversed without a superseding ADR, or an edited accepted ADR.
- Hand edits under `docs/reference/`.
- Exported names without doc comments; `TODO` without an issue number.
- `fmt.Print*`, `log.*`, `io/ioutil`, `pkg/errors`, or testify.
- Commit trailers naming an AI tool, or `Claude-Session:`.

## Do not flag

- Formatting, import order, and anything `make check` already enforces.
- Style preferences that docs/style.md does not state.
- Test helper naming or table layout that follows the surrounding code.
- Missing features listed under [Not in scope](../README.md#not-in-scope) in the README, or ruled out by an ADR.
- Generated files under `docs/reference/` and `docs/decisions/index.md` when `make docs` produces them.
