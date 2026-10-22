# 0019. Document only install channels with an update path

Status: Accepted

Date: 2026-10-22

## Context

The README lists the ways to install secretref. Each release builds more than the README lists. GoReleaser produces `.tar.gz` and `.zip` archives, `.deb` and `.rpm` packages, and checksums signed with cosign.

A channel the README documents is one users will rely on. A user who installs from a one-off `.deb` or `.rpm`, or from an archive, gets no new versions unless they go back to the Releases page. secretref resolves secrets, so a user stuck on an old version also misses its security fixes.

A `curl … | sh` install script was also considered. It runs remote code in the user's shell. For a tool whose promise is that no secret value leaks, that is a poor first step. It would also have to detect six OS and architecture targets correctly and be kept up to date.

## Decision

The README documents only install channels with an update path: Homebrew, Scoop, npm, and `go install`. A new channel is added when users can update through it, as with winget once it is published.

Standalone `.deb` and `.rpm` downloads, release archives, and an install script are not documented as install options. The release assets stay on the Releases page.

## Consequences

- Every documented way to install secretref can also update it.
- Linux users without Homebrew, npm, or Go have no documented option. Hosting an apt or dnf repository would give them one; that needs a new record.
- Users can still download release assets and verify them against the signed checksums without being told to.
