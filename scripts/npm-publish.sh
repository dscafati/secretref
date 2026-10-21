#!/usr/bin/env bash
# Publish the secretref npm packages from GoReleaser output in dist/.
# One @secretref/<os>-<cpu> package per binary, then the secretref launcher.
# Uses npm trusted publishing (OIDC); versions npm already has are skipped.
# DRY_RUN=true runs `npm publish --dry-run` and publishes nothing.
#
# Trusted publishing cannot create a package: each one must already exist on
# npm with a trusted publisher configured. Every package is checked before any
# is published. `npm-publish.sh preflight` runs only that check and needs no
# dist/, so the release workflow can stop before GoReleaser publishes anything.
# It also checks that npm is new enough for trusted publishing.
# docs/releasing.md describes the one-time bootstrap.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dry_run="${DRY_RUN:-false}"

# package_exists NAME: 0 if npm has the package, 1 on E404, exits on other errors.
package_exists() {
  local name="$1" out
  if out="$(npm view "$name" name 2>&1)"; then
    return 0
  fi
  if printf '%s\n' "$out" | grep -q E404; then
    return 1
  fi
  echo "npm view $name failed:" >&2
  printf '%s\n' "$out" >&2
  exit 1
}

# npm_supports_oidc: fail unless npm can do trusted publishing (11.5.1 or later).
# The release workflow uses the npm that ships with Node instead of installing one.
npm_supports_oidc() {
  local v
  v="$(npm --version)"
  if [ "$(printf '%s\n' 11.5.1 "$v" | sort -V | head -n 1)" != 11.5.1 ]; then
    echo "npm $v is older than 11.5.1, which trusted publishing needs." >&2
    echo "Use a Node release that bundles a newer npm." >&2
    exit 1
  fi
}

# preflight: fail unless npm supports trusted publishing and every package
# already exists on npm. The names come from the launcher's
# optionalDependencies plus the launcher itself.
preflight() {
  local name missing=()
  npm_supports_oidc
  while read -r name; do
    package_exists "$name" || missing+=("$name")
  done < <(jq -r '.optionalDependencies | keys[]' "$root/npm/secretref/package.json"; echo secretref)
  if [ "${#missing[@]}" -eq 0 ]; then
    echo "npm preflight: all packages exist"
    return 0
  fi
  echo "These packages do not exist on npm yet: ${missing[*]}" >&2
  echo "Trusted publishing (OIDC) cannot create a package. Bootstrap each one by" >&2
  echo "hand and configure its trusted publisher first; see docs/releasing.md." >&2
  if [ "$dry_run" = "true" ]; then
    echo "DRY_RUN=true: continuing; a real release would stop here." >&2
    return 0
  fi
  exit 1
}

if [ "${1:-}" = preflight ]; then
  preflight
  exit 0
fi

dist="${DIST_DIR:-$root/dist}"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
version="$(jq -r .version "$dist/metadata.json")"

publish() {
  local dir="$1" name="$2"
  if [ "$dry_run" != "true" ] && npm view "$name@$version" version >/dev/null 2>&1; then
    echo "skip $name@$version: already published"
    return 0
  fi
  local flags=(--access public)
  case "$version" in
    *-*) flags+=(--tag next) ;;
  esac
  if [ "$dry_run" = "true" ]; then
    flags+=(--dry-run)
  else
    flags+=(--provenance)
  fi
  (cd "$dir" && npm publish "${flags[@]}")
}

# Stage one directory per platform package. Nothing is published yet.
platforms=()
while IFS=$'\t' read -r goos goarch path; do
  case "$goos" in
    windows) os=win32; exe=secretref.exe ;;
    linux | darwin) os="$goos"; exe=secretref ;;
    *) echo "unsupported goos: $goos" >&2; exit 1 ;;
  esac
  case "$goarch" in
    amd64) cpu=x64 ;;
    arm64) cpu=arm64 ;;
    *) echo "unsupported goarch: $goarch" >&2; exit 1 ;;
  esac
  platform="$os-$cpu"
  pkg="$stage/$platform"
  mkdir -p "$pkg/bin"
  case "$path" in
    /*) src="$path" ;;
    *) src="$root/$path" ;;
  esac
  [ -f "$src" ] || { echo "binary not found: $src" >&2; exit 1; }
  cp "$src" "$pkg/bin/$exe"
  cp "$root/LICENSE" "$pkg/LICENSE"
  chmod +x "$pkg/bin/$exe"
  sed -e "s/@@PLATFORM@@/$platform/g" -e "s/@@VERSION@@/$version/g" \
      -e "s/@@OS@@/$os/g" -e "s/@@CPU@@/$cpu/g" \
      "$root/npm/platform/package.json.tmpl" >"$pkg/package.json"
  platforms+=("$platform")
done < <(jq -r '.[] | select(.type == "Binary") | [.goos, .goarch, .path] | @tsv' "$dist/artifacts.json")

# The launcher's optionalDependencies must name exactly the staged platforms,
# or it would depend on versions that never get published.
want="$(jq -r '.optionalDependencies | keys[]' "$root/npm/secretref/package.json" | sort)"
got="$(printf '@secretref/%s\n' "${platforms[@]}" | sort)"
if [ "$want" != "$got" ]; then
  echo "npm/secretref/package.json optionalDependencies do not match the binaries in $dist:" >&2
  diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") >&2 || true
  exit 1
fi

preflight

for platform in "${platforms[@]}"; do
  publish "$stage/$platform" "@secretref/$platform"
done

launcher="$stage/secretref"
mkdir -p "$launcher"
cp -R "$root/npm/secretref/." "$launcher/"
cp "$root/LICENSE" "$launcher/LICENSE"
jq --arg v "$version" \
  '.version = $v | .optionalDependencies |= with_entries(.value = $v)' \
  "$root/npm/secretref/package.json" >"$launcher/package.json"
publish "$launcher" secretref
