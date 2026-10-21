# Releasing

This page is for maintainers. Pushing a `v*` tag runs
[`.github/workflows/release.yml`](../.github/workflows/release.yml) in the
`release` environment. In order, the job:

1. Checks that all seven npm packages exist (`scripts/npm-publish.sh preflight`).
   If one is missing it stops here, before anything is published.
2. Runs GoReleaser (`.goreleaser.yaml`). GoReleaser builds the six
   `CGO_ENABLED=0` binaries, archives, `.deb` and `.rpm` packages, checksums, a
   cosign signature for the checksums, and SBOMs. It publishes the GitHub
   release, pushes the Homebrew cask to `dscafati/homebrew-tap` and the Scoop
   manifest to `dscafati/scoop-bucket`, and opens a pull request on
   `microsoft/winget-pkgs` from the `dscafati/winget-pkgs` fork.
3. Attests build provenance for the archives, packages, and checksums.
4. Publishes `@secretref/<os>-<cpu>` for each binary, then the `secretref`
   launcher, with npm trusted publishing (`scripts/npm-publish.sh`).

Prerelease tags such as `v0.1.0-rc.1` are marked as prereleases on GitHub and
go to npm under the `next` dist-tag. They skip the Homebrew, Scoop, and winget
uploads (`skip_upload: auto`); GoReleaser writes those manifests to `dist/`
only.

## One-time setup

Do these once, in this order, before the first tag.

### GitHub repository

- Create the `release` environment under Settings > Environments. Limit its
  deployment branches and tags to the `v*` tag pattern. Adding yourself as a
  required reviewer is optional; it makes every release wait for approval.
- Enable Discussions under Settings > General > Features. The issue chooser
  links "Questions and ideas" to `https://github.com/dscafati/secretref/discussions`,
  which is a 404 until Discussions is on.

### Package-manager repositories

These must exist before the first stable release, or GoReleaser fails after
the GitHub release is already published.

- `dscafati/homebrew-tap`: a public repository with at least one commit on the
  default branch. GoReleaser writes `Casks/secretref.rb`.
- `dscafati/scoop-bucket`: a public repository with at least one commit on the
  default branch. GoReleaser writes `secretref.json` at the root.
- `dscafati/winget-pkgs`: a fork of `microsoft/winget-pkgs`. GoReleaser syncs it,
  pushes a `secretref-<version>` branch, and opens the pull request upstream.

### `TAP_GITHUB_TOKEN`

Add it as a secret of the `release` environment, not the repository, so only
the release job can read it. One token covers all three repositories above.

Use a classic personal access token with the `public_repo` scope. A
fine-grained token can push to your own repositories, but GitHub documents
that fine-grained tokens cannot contribute to public repositories you are not
a member of, so the pull request on `microsoft/winget-pkgs` would fail. `public_repo` grants write access to every
public repository you own; give the token an expiry and rotate it.

The release job's own `GITHUB_TOKEN` creates the GitHub release; it needs no
setup.

### npm

npm trusted publishing (OIDC) cannot create a package. A trusted publisher can
only be configured on a package that already exists, so each package needs one
manual publish first. The launcher, `secretref`, already exists (versions
`0.0.1` and `0.0.0-stage`). None of the six `@secretref/*` packages exist, and
as of October 2026 the `@secretref` scope is unclaimed.

You need an npm account with two-factor authentication enabled.

1. Create the npm organization `secretref` on npmjs.com (the free plan, for
   public packages). Until it exists, nobody can publish under `@secretref/`.
   Anyone else can claim the name, so do this first.

2. Publish a placeholder `0.0.0` of each platform package by hand. The launcher
   pins exact versions, so no install ever resolves a placeholder.

   ```sh
   npm login
   for p in linux-x64 linux-arm64 darwin-x64 darwin-arm64 win32-x64 win32-arm64; do
     d="$(mktemp -d)"
     cat >"$d/package.json" <<EOF
   {
     "name": "@secretref/$p",
     "version": "0.0.0",
     "description": "Placeholder that reserves the name. Install secretref instead.",
     "license": "Apache-2.0",
     "repository": { "type": "git", "url": "git+https://github.com/dscafati/secretref.git" }
   }
   EOF
     (cd "$d" && npm publish --access public)
   done
   ```

3. Configure the trusted publisher on all seven packages: the six above and
   `secretref`. Each one must name the repository, the workflow file, and the
   environment exactly, or npm rejects the publish.

   | Field | Value |
   |---|---|
   | Publisher | GitHub Actions |
   | Organization or user | `dscafati` |
   | Repository | `secretref` |
   | Workflow filename | `release.yml` |
   | Environment | `release` |

   Do it in each package's Settings > Trusted Publisher on npmjs.com, or from
   the CLI (`npm trust` needs npm 11.15.0 or later):

   ```sh
   for pkg in @secretref/linux-x64 @secretref/linux-arm64 @secretref/darwin-x64 \
              @secretref/darwin-arm64 @secretref/win32-x64 @secretref/win32-arm64 secretref; do
     npx -y npm@11.21.1 trust github "$pkg" \
       --repo dscafati/secretref --file release.yml --env release --allow-publish
   done
   ```

   A new trusted publisher configuration expires if it has no successful
   publish within 2 days, and an expired one cannot be edited. Configure the
   trusted publishers right before you push the first tag. If a configuration
   expires, delete it and create it again.

4. After the first release publishes through OIDC, set each of the seven
   packages to "Require two-factor authentication and disallow tokens" under
   Settings > Publishing access. Trusted publishing keeps working.

5. Once a stable version is out, deprecate the old placeholders so they do not
   show up as candidates:

   ```sh
   npm deprecate secretref@0.0.1 "Placeholder. Install the latest secretref."
   npm deprecate secretref@0.0.0-stage "Placeholder. Install the latest secretref."
   for p in linux-x64 linux-arm64 darwin-x64 darwin-arm64 win32-x64 win32-arm64; do
     npm deprecate "@secretref/$p@0.0.0" "Placeholder. Install secretref."
   done
   ```

   `latest` on `secretref` stays at `0.0.1` until the first stable release,
   because prereleases publish under `next`.

## Dry run

Run the release workflow by hand (Actions > release > Run workflow, with
`dry_run` checked). It builds a snapshot with signing and SBOMs, skips every
GitHub, tap, bucket, and winget publish, and runs `npm publish --dry-run`. The
npm preflight only warns in a dry run, so it works before the npm bootstrap.

Locally, `make snapshot` builds the same artifacts and manifests into `dist/`
without signing or SBOMs, because those need cosign and syft on `PATH`.

## Each release

1. Run the `release` skill (`.agents/skills/release/SKILL.md`) or do its steps
   by hand: a clean tree on `main`, `make check`, `make build-all`,
   `make snapshot`, and a look at `dist/homebrew/Casks/secretref.rb`,
   `dist/scoop/secretref.json`, and `dist/winget/`.
2. Optionally run the release workflow in dry-run mode on `main`.
3. Tag and push. Use `vX.Y.Z-rc.N` for a prerelease.

   ```sh
   git tag -a vX.Y.Z -m vX.Y.Z
   git push origin vX.Y.Z
   ```

4. Approve the `release` environment deployment if you set a required reviewer.
5. Check the results:
   - The GitHub release has the archives, packages, `checksums.txt`,
     `checksums.txt.sigstore.json`, and the SBOMs.
   - `npm view secretref dist-tags` shows the new version under `latest`, or
     under `next` for a prerelease.
   - Stable releases only: the tap and bucket have a new commit, and the winget
     pull request is open. A winget maintainer reviews and merges it.

## When something fails

- The npm preflight fails: a package is missing. Nothing was published.
  Complete the npm steps above, then re-run the job.
- `npm publish` fails with an authentication error: the trusted publisher does
  not match (repository, `release.yml`, or `release` environment), or its
  configuration expired before its first publish. GoReleaser has already
  published by then, so see the next item before re-running.
- Any step fails after GoReleaser published: the GitHub release and its assets
  are public, and re-running the job runs GoReleaser again, which can fail on
  the existing assets. Delete the GitHub release (keep the tag) and re-run.
  The npm step skips versions npm already has. For a stable release, check
  whether the winget pull request was already opened and close the duplicate.
