#!/bin/sh
# Fail if a decision record is not linked from docs/decisions/index.md.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
dir="$root/docs/decisions"
status=0

for path in "$dir"/[0-9][0-9][0-9][0-9]-*.md; do
	[ -e "$path" ] || continue
	name=$(basename "$path")
	if ! grep -F "($name)" "$dir/index.md" >/dev/null; then
		echo "not listed in docs/decisions/index.md: $name" >&2
		status=1
	fi
done

exit "$status"
