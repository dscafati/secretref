---
name: release
description: Use when preparing a secretref release. Prepares and verifies; a human tags, pushes, and publishes.
---

# Prepare a release

You prepare and verify. You do not push, tag on a remote, publish to npm, or change GitHub
or npm settings. `AGENTS.md` requires human approval for those.

[`docs/releasing.md`](../../../docs/releasing.md) is the maintainer runbook: what the
release workflow does, the one-time GitHub and npm bootstrap, and the per-release steps.
Read it first.

1. Read `AGENTS.md`, `docs/releasing.md`, `.github/workflows/release.yml`,
   `.goreleaser.yaml`, and `scripts/npm-publish.sh`.
2. Confirm the working tree is clean and `make check` and `make build-all` pass.
3. Run `scripts/npm-publish.sh preflight`. If it reports missing packages, the npm
   bootstrap in `docs/releasing.md` is not done; tell the maintainer, because a real
   release would stop at that step.
4. Run `make snapshot` and confirm `dist/` has an archive for every target in `AGENTS.md`,
   plus `.deb` and `.rpm` packages.
5. Inspect `dist/homebrew/Casks/secretref.rb` (binary, manpage, completions, quarantine
   postflight), `dist/scoop/secretref.json`, and `dist/winget/`.
6. Verify the archives contain no resolved values or credentials and that
   `secretref version` reports the expected string.
7. Run `DRY_RUN=true scripts/npm-publish.sh` and confirm it stages all six platform packages
   and the launcher at the snapshot version.
8. Draft the release notes from Conventional Commit subjects since the last tag.
9. Hand the maintainer the tag command and the "Each release" checklist from
   `docs/releasing.md`, then stop.
