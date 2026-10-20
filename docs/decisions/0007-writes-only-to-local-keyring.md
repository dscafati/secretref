# 0007. Writes only to the local keyring

Status: Accepted

Date: 2026-10-20

## Context

A developer needs somewhere to keep a personal secret without a cloud account. Writing to AWS or GCP from a developer tool adds permissions risk, and cloud consoles and CLIs already do that job well.

## Decision

`secretref set` and `secretref delete` accept only `keyring://` references. Other schemes fail with a usage error (exit 2). `set` reads the value with hidden terminal input, or from stdin when stdin is not a terminal.

The `Writer` interface exists, and only the keyring provider implements it.

## Consequences

- secretref never needs write permission on a cloud secret store.
- The blast radius of a mistaken command is the current user's own keyring.
- Users create cloud secrets with their cloud's own tools.
- Adding write support for another provider needs a new record.
