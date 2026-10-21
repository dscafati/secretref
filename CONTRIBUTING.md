# Contributing to secretref

Thanks for helping. Read this page before you open a pull request.

## Setup

You need Go 1.26. Dev tools run through `go tool -modfile=tools/go.mod`, so you
install nothing globally.

```sh
git clone https://github.com/dscafati/secretref
cd secretref
make check
```

`make check` runs formatting, lint, `go mod tidy` checks, unit tests,
acceptance tests, the generated-docs check, the decision-index check, and the
skills-link check. It does not run fuzz or integration tests. Run it before every
push.

## Tests

| Layer | Command |
|---|---|
| Everything except fuzz and integration | `make check` |
| Fix formatting and lint | `make fix` |
| Lint only | `make lint` |
| Unit and golden tests | `make test` |
| Acceptance tests with the real binary | `make test-acceptance` |
| Fuzz the parsers | `make fuzz` |
| Integration tests | `make test-integration` |
| Regenerate reference docs | `make docs` |
| Build release binaries locally | `make snapshot` |

Integration tests start LocalStack and other containers. Podman works:

```sh
make test-integration CONTAINER=podman
```

Integration tests never use real cloud accounts. A maintainer runs those by hand.

New behavior needs a test first. Tests that touch secrets use a canary value
and fail if it appears anywhere except the child environment.

## Pull requests

- Use a [Conventional Commits](https://www.conventionalcommits.org/) title, for
  example `feat(envfile): support export prefix`. CI checks it.
- Fill in the pull request template.
- Do not add AI co-author trailers. See [AI_POLICY.md](AI_POLICY.md).
- Open an issue first for anything larger than a small fix. Only work on issues
  labeled `help wanted`, unless a maintainer says otherwise in the issue.

## Decision records

Architectural decisions live in [docs/decisions](docs/decisions/index.md).

- To change a decision, write a new record that supersedes the old one. Do not
  edit the old record, except to set its Status line to `Superseded by NNNN`.
- Add the new record to `docs/decisions/index.md`. `make check` fails if you do
  not.
- Use the next free number and the same format as the existing records. If
  another branch merges a record with the same number first, renumber yours
  before merging. `make check` fails on duplicate numbers.

## Security

Do not report vulnerabilities in public issues. Follow [SECURITY.md](SECURITY.md).

## Conduct

This project follows the [Code of Conduct](CODE_OF_CONDUCT.md).
