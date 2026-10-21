# Contributing to secretref

Thanks for helping. Read this page before you open a pull request.

## Setup

You need:

- Go 1.26 or later (the version in `go.mod`),
- `make`,
- a POSIX shell.

On Windows, use Git Bash and install `make`. The repository contains a
symlink, `.claude/skills`. Turn on Developer Mode and enable symlinks in git
before you clone:

```sh
git config --global core.symlinks true
```

Dev tools run through `go tool -modfile=tools/go.mod`, so you install nothing
globally.

```sh
git clone https://github.com/dscafati/secretref
cd secretref
make hooks
make check
```

`make hooks` installs the git hooks. Before each commit they format staged
Go files and scan staged changes for secrets. They reject AI co-author and
`Claude-Session` trailers in the commit message, and before each push they run
the linter. CI runs the same trailer check (`scripts/check-commit-msg.sh`) on
every pull request and every push to `main`.

`make check` runs formatting, lint, `go mod tidy` checks, unit tests,
acceptance tests, the generated-docs check, the decision checks, the skills-link check, the
`doc.go` check, and the tests of the commit-message scripts, and the Reveal-comment check. It does not run fuzz or integration tests. Run it before every
push.

## Tests

| Layer | Command |
|---|---|
| Everything except fuzz and integration | `make check` |
| Fix formatting and lint | `make fix` |
| Lint only | `make lint` |
| Unit and golden tests | `make test` |
| Acceptance tests with the real binary | `make test-acceptance` |
| Fuzz the parsers (CI runs 30 seconds per target) | `make fuzz` |
| Integration tests | `make test-integration` |
| Build every release target with `CGO_ENABLED=0` | `make build-all` |
| Regenerate reference docs | `make docs` |
| Build release binaries locally, without signing or SBOMs | `make snapshot` |

Integration tests start a [Moto](https://github.com/getmoto/moto) container
as the Secrets Manager emulator for `aws-sm`, and run the keyring contract
tests. They need podman or docker, `bash`, and `curl`. `CONTAINER` selects the
container runtime:

```sh
make test-integration CONTAINER=docker
```

On Linux, install `gnome-keyring` and `dbus` (for `dbus-run-session`) and the
keyring tests run against a throwaway keyring on a private session bus.
Without them they use your session's keyring, and they clean up after
themselves. On macOS and Windows they use the OS keyring.

`gcp-sm` has no integration test. Its unit tests run against a fake HTTP
server. Tests never use real cloud accounts. A maintainer runs those by hand.

`make build-all` builds linux, darwin, and windows on amd64 and arm64 with
`CGO_ENABLED=0`. CI runs it too.

New behavior needs a test first. Tests that touch secrets use a canary value
and fail if it appears anywhere except the child environment.

## Pull requests

- Use a [Conventional Commits](https://www.conventionalcommits.org/) title, for
  example `feat(envfile): support export prefix`. CI checks it.
- Fill in the pull request template.
- Do not add AI co-author trailers. See [AI_POLICY.md](AI_POLICY.md).
- Open an issue first for anything larger than a small fix. Only work on issues
  labeled `help wanted`, unless a maintainer says otherwise in the issue.

## Stability

Before 1.0, a minor release may contain breaking changes. The annotation
syntax, the reference grammar, and the exit codes are the exception. A change
to any of them needs a new decision record and a callout in the release notes.
See the [README](README.md#stability).

## Decision records

Architectural decisions live in [docs/decisions](docs/decisions/index.md).

- To change a decision, write a new record that supersedes the old one. Do not
  edit the old record, except to set its Status line to `Superseded by NNNN`.
- Run `make docs` to regenerate `docs/decisions/index.md`. Do not edit it by
  hand. `make check` fails if it is out of date.
- Use the next free number and the same format as the existing records. If
  another branch merges a record with the same number first, renumber yours
  before merging. `make check` fails on duplicate numbers, on index links that
  do not resolve, and on an edit to an existing record other than its Status
  line. The last check compares with the merge-base with `origin/main` (or
  `main`), or with `HEAD~1` when neither exists. CI sets `ADR_BASE` to the pull
  request base or the previous tip of `main`.

## Releases

Maintainers cut releases. See [docs/releasing.md](docs/releasing.md).

## Security

Do not report vulnerabilities in public issues. Follow [SECURITY.md](SECURITY.md).

## Conduct

This project follows the [Code of Conduct](CODE_OF_CONDUCT.md).
