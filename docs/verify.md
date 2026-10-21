# Verifying a release

Every release on [GitHub Releases](https://github.com/dscafati/secretref/releases)
is built by the `release` workflow from a `v*` tag. You can check that a file
came from that workflow in three ways:

- the cosign signature on `checksums.txt`, then the checksums,
- the GitHub build provenance attestation,
- for npm, the registry signatures and provenance.

The examples use version `0.1.0` on Linux amd64. Replace the version and the
platform with yours.

## Download

```sh
VERSION=0.1.0
gh release download "v$VERSION" --repo dscafati/secretref \
  --pattern checksums.txt \
  --pattern checksums.txt.sigstore.json \
  --pattern "secretref_${VERSION}_linux_amd64.tar.gz"
```

You can also download the files from the release page in a browser.

Archives are named `secretref_<version>_<os>_<arch>.tar.gz`, or `.zip` on
Windows. The `.deb` and `.rpm` packages are on the same page.

## Signature and checksums

`checksums.txt` lists the SHA-256 of the release files. It is signed with
[cosign](https://docs.sigstore.dev/cosign/) keyless signing, and the signature
is in the bundle `checksums.txt.sigstore.json`.

1. Verify the signature:

   ```sh
   cosign verify-blob \
     --bundle checksums.txt.sigstore.json \
     --certificate-identity-regexp '^https://github\.com/dscafati/secretref/\.github/workflows/release\.yml@refs/tags/v' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     checksums.txt
   ```

   cosign prints `Verified OK` on success. The identity says the signature was
   made by `release.yml` in `dscafati/secretref`, running for a `v*` tag. The
   issuer says the identity came from GitHub Actions.

2. Check the files you downloaded against the signed list:

   ```sh
   sha256sum --ignore-missing -c checksums.txt
   ```

   On macOS:

   ```sh
   grep " secretref_${VERSION}_darwin_arm64.tar.gz\$" checksums.txt | shasum -a 256 -c
   ```

   On Windows, compare the output of
   `Get-FileHash secretref_0.1.0_windows_amd64.zip -Algorithm SHA256` with the
   line in `checksums.txt`. The case of the hex digits does not matter.

## Build provenance

The release workflow attests the archives, the `.deb` and `.rpm` packages, and
`checksums.txt` with GitHub build provenance. With the
[GitHub CLI](https://cli.github.com/):

```sh
gh attestation verify "secretref_${VERSION}_linux_amd64.tar.gz" --repo dscafati/secretref
```

To also require the release workflow as the signer, add
`--signer-workflow dscafati/secretref/.github/workflows/release.yml`.

## SBOMs

Each archive has a software bill of materials in SPDX JSON, named after the
archive with `.sbom.json` added, for example
`secretref_0.1.0_linux_amd64.tar.gz.sbom.json`. Download it from the same
release. If `checksums.txt` lists it, `sha256sum --ignore-missing -c
checksums.txt` checks it too.

## npm

The npm packages `secretref` and `@secretref/<os>-<cpu>` are published from
the release workflow with npm provenance. To check the registry signatures and
provenance attestations of an installed copy:

```sh
mkdir secretref-verify && cd secretref-verify
npm install secretref
npm audit signatures
```

`npm audit signatures` checks every package in the project, including the
platform package that holds the binary. The package page on npmjs.com also
shows the provenance and links to the workflow run.

## Homebrew, Scoop, and winget

These install the same archives from GitHub Releases and check them against
the checksum in the formula or manifest. To check the signature yourself,
download the archive and follow the steps above.
