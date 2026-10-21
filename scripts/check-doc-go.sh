#!/bin/sh
# Fails when a directory with Go files has no doc.go (docs/style.md). Walks the
# tree instead of asking `go list`, so packages behind build tags are covered.
set -eu
cd "$(dirname "$0")/.."
status=0
for dir in $(find . -name '*.go' -not -path './tools/*' -not -path '*/testdata/*' -not -path './.*' -exec dirname {} \; | sort -u); do
  if [ ! -f "$dir/doc.go" ]; then
    echo "missing doc.go: $dir" >&2
    status=1
  fi
done
exit "$status"
