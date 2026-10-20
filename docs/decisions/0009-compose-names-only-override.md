# 0009. Docker Compose support through a names-only override

Status: Accepted

Date: 2026-10-20

## Context

Many projects load `.env` files through Docker Compose `env_file:`. Compose reads those files itself, so secretref's environment does not reach containers. Editing `compose.yaml` to fix this would break the goal of changing nothing in the repo.

## Decision

When the command is `docker compose ...` (Compose v2), secretref finds the compose files, finds services whose `env_file` includes a file it loaded, and writes a temporary override file with mode 0600. The override contains only names: `services.<svc>.environment.<KEY>: ${KEY}`. secretref removes it after the child exits.

Compose substitutes `${KEY}` from its own environment, which secretref set. The `environment:` key takes precedence over `env_file:`. The override never contains a value. podman-compose and `docker-compose` v1 are not supported in v0.1.

## Consequences

- `compose.yaml` stays unchanged.
- No resolved value reaches disk, in line with ADR 0002.
- secretref stays running as the parent in compose mode, to remove the override and forward signals.
- Compose file discovery must follow Compose's own rules, so it needs careful tests.
