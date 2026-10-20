# 0005. Go with pure-Go dependencies, no CGO

Status: Accepted

Date: 2026-10-20

## Context

secretref ships as a single static binary for Linux, macOS, and Windows on amd64 and arm64. CGO makes cross-compilation hard, ties binaries to a C library, and complicates release automation.

## Decision

We write secretref in Go and build with `CGO_ENABLED=0`. Every dependency must be pure Go. This rules out libraries that need a C toolchain or a system library at build time.

The keyring provider uses `zalando/go-keyring`, which talks to the OS through D-Bus, the macOS `security` tool, and the Windows credential API without CGO.

## Consequences

- GoReleaser builds all six targets from one machine.
- Users download one file with no runtime dependencies.
- Some libraries are unavailable. We pick alternatives or write a small client.
- CI must build with `CGO_ENABLED=0` so a CGO dependency fails fast.
