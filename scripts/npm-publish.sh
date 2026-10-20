#!/usr/bin/env bash
# Publish the secretref npm packages from GoReleaser output in dist/.
# One @secretref/<os>-<cpu> package per binary, then the secretref launcher.
# Uses npm trusted publishing (OIDC); versions npm already has are skipped.
# DRY_RUN=true runs `npm publish --dry-run` and publishes nothing.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dist="${DIST_DIR:-$root/dist}"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

version="$(jq -r .version "$dist/metadata.json")"
dry_run="${DRY_RUN:-false}"

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

jq -r '.[] | select(.type == "Binary") | [.goos, .goarch, .path] | @tsv' "$dist/artifacts.json" |
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
    publish "$pkg" "@secretref/$platform"
  done

launcher="$stage/secretref"
mkdir -p "$launcher"
cp -R "$root/npm/secretref/." "$launcher/"
cp "$root/LICENSE" "$launcher/LICENSE"
jq --arg v "$version" \
  '.version = $v | .optionalDependencies |= with_entries(.value = $v)' \
  "$root/npm/secretref/package.json" >"$launcher/package.json"
publish "$launcher" secretref
