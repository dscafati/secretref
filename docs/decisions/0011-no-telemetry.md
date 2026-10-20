# 0011. No telemetry

Status: Accepted

Date: 2026-10-20

## Context

secretref handles secret references. Users must be able to trust that it sends nothing anywhere except to the secret stores they point it at.

## Decision

secretref collects no telemetry, usage data, or crash reports. It makes network calls only to the providers named in the user's own references. It does not check for updates.

## Consequences

- Users can audit the network behavior by reading the provider code.
- We have no usage data to guide the roadmap. We rely on issues and discussions.
- Adding any telemetry, even opt-in, needs a new record that supersedes this one.
