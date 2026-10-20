---
name: release
description: Use when preparing a secretref release. Prepares and verifies; a human tags, pushes, and publishes.
---

# Prepare a release

You prepare and verify. You do not push, tag on a remote, publish to npm, or change GitHub
settings. `AGENTS.md` requires human approval for those.

1. Read `AGENTS.md`, `CONTRIBUTING.md`, and the release workflow and GoReleaser config
   under `.github/workflows/` and the repository root.
2. Confirm the working tree is clean and `make check` passes.
3. Run `make snapshot` and confirm every target in `AGENTS.md` produced an artifact.
4. Verify the archives contain no resolved values or credentials and that
   `secretref version` reports the expected string.
5. Check npm package templates match the version, following `scripts/` and the npm section
   of the release workflow.
6. Draft the release notes from Conventional Commit subjects since the last tag.
7. Hand the maintainer the tag command and the checklist of manual GitHub steps, then stop.
