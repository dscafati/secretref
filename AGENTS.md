# AGENTS.md

secretref is a Go CLI. A developer annotates a `.env` file with a comment naming where
a value lives; `secretref run -- <cmd>` resolves it and starts the command with the real
value in its environment. Single static binary, `CGO_ENABLED=0`, Apache-2.0, no telemetry.

## Commands

Run `make` from the repository root. The Makefile is the source of truth for targets.

| Command | Purpose |
|---|---|
| `make check` | Everything CI runs on the fast path. Must pass before you report done. |
| `make fix` | Mechanical fixes only: gofumpt and linter autofixes. |
| `make test` | Unit tests with `-race`. |
| `make test-acceptance` | Real binary through testscript against `provider/fake`. |
| `make fuzz` | Parser fuzz targets; `FUZZTIME` sets the duration. |
| `make test-integration` | Providers against LocalStack and a GCP fake; `CONTAINER` selects podman or docker. |
| `make docs` | Regenerate `docs/reference/` from cobra. |
| `make snapshot` | GoReleaser snapshot build for all targets. |

Dev tools are pinned in `tools/go.mod` and run as `go tool -modfile=tools/go.mod <tool>`.
Never install tools globally and never edit tool versions anywhere else.

## Invariants

- No resolved value in stdout, stderr, logs, error text, or any file. Ever.
- Exit codes: the child's code; 1 internal; 2 usage or no env file; 3 resolution failed; 127 command not found.
- Defaults: env files `.env` then `.env.local`; concurrency 8; timeout 30s.
- The existing process environment wins over file values and is never resolved.
- `set` and `delete` accept only `keyring://`.
- Providers in v0.1: `aws-sm`, `gcp-sm`, `keyring`. Nothing else.
- `CGO_ENABLED=0` must build for linux, darwin, windows on amd64 and arm64.
- Stdout belongs to the child process. Diagnostics go to stderr through `log/slog`.
- No package-level variables except `build.Version`. Dependencies are passed to constructors.

## Rules

- Do not hand-edit `docs/reference/`; run `make docs`.
- Do not write tests that call real clouds. Those need maintainer credentials and run manually.
- Stop and ask a human before `git push` or opening a pull request.
- Commits use Conventional Commits. No AI `Co-Authored-By` or `Claude-Session` trailers.
- Never bypass hooks with `--no-verify`.
- Reversing a decision needs a new ADR that supersedes the old one. Do not edit accepted records.
- Fix a bug by writing the failing test first.
- One responsibility per file. No `utils` or `common` packages.
- Do not create `docs/specs/` or `docs/superpowers/`.

## Read first

| Task | Read |
|---|---|
| Writing or reviewing any Go code | [docs/style.md](docs/style.md) |
| Why something is built this way | [docs/decisions/index.md](docs/decisions/index.md) |
| Changing a decision | [docs/decisions/](docs/decisions/) and [CONTRIBUTING.md](CONTRIBUTING.md) |
| Opening a change | [CONTRIBUTING.md](CONTRIBUTING.md), [AI_POLICY.md](AI_POLICY.md) |
| Security-sensitive behavior | [SECURITY.md](SECURITY.md) |
| Reviewing a diff | [.github/review-rules.md](.github/review-rules.md) |
| Adding a provider | `.agents/skills/add-provider/SKILL.md` |
| Adding an acceptance test | `.agents/skills/write-acceptance-test/SKILL.md` |
| Before opening a PR | `.agents/skills/pre-pr-review/SKILL.md` |
| Cutting a release | `.agents/skills/release/SKILL.md` |

Skills live in `.agents/skills/`; `.claude/skills` is a symlink to it.
