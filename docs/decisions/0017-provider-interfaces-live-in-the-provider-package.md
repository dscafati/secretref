# 0017. Provider interfaces live in the provider package

Status: Accepted

Date: 2026-10-21

## Context

[0015](0015-manual-dependency-injection-immutable-globals.md) says providers implement small interfaces, `Provider` and `Writer`, "defined next to their consumers". `docs/style.md` repeats the rule for all interfaces.

The code does not follow it for these two. `internal/provider` declares `Provider` and `Writer`, and several packages use them: the CLI (`root.go`, `set.go`), the wiring in `cmd/secretref`, `provider/fake`, `provider/providertest`, and each real provider, which asserts at compile time that it satisfies them. There is no single consumer to own the definition. Copying the interface into every consumer would let the copies drift.

## Decision

`provider.Provider` and `provider.Writer` stay in `internal/provider`. Several consumers and every implementation share them, so one definition is the contract.

The default for every other interface does not change: define it in the package that consumes it, keep it small, and do not export it from the implementing package.

[0015](0015-manual-dependency-injection-immutable-globals.md) is otherwise unchanged. This record only moves where those two interfaces are defined.

## Consequences

- The written rule matches the code, and a reviewer no longer has to guess whether the provider package is an exception.
- A new provider method is a change to a shared interface and needs a look at every implementation.
- A consumer that needs only part of `Provider` may still declare a narrower interface of its own.
