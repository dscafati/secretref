# 0002. Resolve at process start; never write values to disk

Status: Accepted

Date: 2026-10-20

## Context

Developers need real secrets in their app's environment. Tools that render secrets to files, or that keep a daemon with a cache, leave plaintext on disk or in memory long after the app starts. The goal is that a resolved value exists only where the app needs it.

## Decision

`secretref run` resolves every annotated reference once, before the command starts. It places the values in the child process environment and starts the command. secretref never writes a resolved value to a file, stdout, stderr, a log, or an error message.

On Unix, secretref replaces itself with the command (`exec`), so no secretref process stays alive. There is no daemon, no cache, and no rendering of values to files. The `secret.Secret` type prints as `[redacted]` and exposes its bytes only through `Reveal()`.

## Consequences

- The attack surface is small. A value lives in the child's environment and nowhere else.
- Values do not refresh while the app runs. Changing a secret needs a restart.
- Tests use a canary value and fail if it appears anywhere except the child environment.
- Apps that need a file on disk are out of scope for v0.1.
- On Windows, and in compose mode, secretref stays running as the parent to forward signals and report the exit code.
