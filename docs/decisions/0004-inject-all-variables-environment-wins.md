# 0004. Inject all variables; the existing environment wins

Status: Superseded by 0016

Date: 2026-10-20

## Context

After secretref starts the app, the app's dotenv loader still runs. Most loaders do not override variables that already exist. CI and production already set real values in the environment and must not be changed or resolved against a secret store.

## Decision

`run` injects every variable from the env files, annotated or not. Variables already present in the process environment win. secretref neither overrides nor resolves them.

Annotated keys that are not already set are resolved. Later files override earlier files, so `.env.local` overrides `.env`.

## Consequences

- CI and production need no change. Their environment already holds real values.
- Loaders configured to override (`override: true`, `Dotenv.overload`, `godotenv.Overload`) replace real values with placeholders. The README documents this.
- Injecting unannotated variables lets Docker Compose and similar tools see the whole file.
- A developer cannot force a secret over an existing environment variable without unsetting it.
