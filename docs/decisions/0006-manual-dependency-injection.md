# 0006. Manual dependency injection

Status: Superseded by 0015

Date: 2026-10-20

## Context

The code needs to swap providers, the clock, and process execution in tests. Dependency injection frameworks and package globals hide wiring and make tests order-dependent.

## Decision

Constructors receive their dependencies. `cmd/secretref/main.go` wires everything. There is no DI framework, no `init()` registration, and no package globals except `build.Version`.

Providers implement small interfaces (`Provider`, `Writer`) defined next to their consumers. Tests pass `provider/fake`.

## Consequences

- Wiring is visible in one file.
- Tests build only the pieces they need.
- Constructors can grow long argument lists. We group related arguments into small structs when that happens.
- Reviewers reject package-level variables other than `build.Version`.
