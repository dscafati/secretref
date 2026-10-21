# 0016. Inject annotated keys by default

Status: Accepted

Date: 2026-10-21

## Context

[0004](0004-inject-all-variables-environment-wins.md) made `run` inject every variable from the env files, annotated or not. Variables already in the environment win.

Dotenv loaders also skip variables that are already in the environment. Frameworks such as Next.js, Vite, and Rails load mode-specific files, for example `.env.development` or `.env.test`, that override `.env`. When secretref injects every value from `.env`, those values are in the environment before the framework starts, so the framework keeps them and ignores the mode-specific file. A project that runs correctly without secretref then behaves differently with it.

Only values that hold secret material need secretref. The app's own loader already reads every other line, and reads it correctly for its mode.

## Decision

By default, `run` gives the command:

- each annotated key whose reference resolved, and
- each plain key whose value expands, through `${...}`, from such a key, directly or through other plain keys. `DATABASE_URL=postgres://app:${DB_PASSWORD}@db/app` is injected when `DB_PASSWORD` resolved, because the app's loader would build it with the placeholder.

It does not inject other plain keys, keys whose reference failed and fell back under `--allow-missing`, or keys already in the environment. The app's dotenv loader reads the plain and fallen-back keys itself.

The flag `--all` injects every variable from the files, as 0004 did.

In both modes, variables already present in the process environment win. secretref neither overrides nor resolves them. Later files override earlier files, so `.env.local` overrides `.env`.

In compose mode the override names exactly the keys that were injected. A key named there but missing from the environment would blank the service's `env_file:` value.

`check` does not change.

This record supersedes 0004.

## Consequences

- Mode-specific dotenv files keep working under secretref.
- An app that reads the environment without a dotenv loader, such as a Go binary or a shell script, sees only the injected keys. It needs `--all` to see plain values.
- Under `--allow-missing`, a key that fell back is not injected. An app with a dotenv loader reads the same file value. An app without one needs `--all`.
- CI and production need no change. Their environment already holds real values.
- Loaders configured to override (`override: true`, `Dotenv.overload`, `godotenv.Overload`) still replace resolved values with placeholders. The user guide documents this.
- Docker Compose containers get plain values from `env_file:` as before, and injected keys through the override.
- This changes `run`'s default output environment. The release notes call it out.
