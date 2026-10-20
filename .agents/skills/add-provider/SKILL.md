---
name: add-provider
description: Use when adding a new secret provider (a new reference scheme) to secretref. Walks through the interfaces, contract tests, docs, and decision record a provider needs.
---

# Add a provider

A new provider changes the product surface, so it needs a decision before code.

1. Read `AGENTS.md` (invariants, rules) and `docs/style.md`. Check `docs/decisions/index.md`:
   if the scheme is not covered by an accepted ADR, write a new ADR first, following the
   format and numbering in that directory, and update the index.
2. Read the `Provider` and `Writer` interfaces and the error kinds in `internal/provider`,
   and the reference grammar in `internal/ref`. Use an existing provider package under
   `internal/provider/` as the model for layout.
3. Write the reference parsing tests first, then the provider tests. The new provider must
   pass the shared contract suite in `internal/provider/providertest`. Add it there; do not
   write a parallel suite.
4. Implement the provider as its own package with a constructor that receives its
   dependencies. Wrap failures in the sentinel error kinds. Errors name the key and ref,
   never the value.
5. Check the dependency is pure Go and `CGO_ENABLED=0` builds on every target listed in
   `AGENTS.md`.
6. Wire it in `cmd/secretref/main.go` only.
7. Add an acceptance case (see `write-acceptance-test`) using the canary rule.
8. Run `make docs` to regenerate reference docs, update the README reference table, then
   run `make check`.
