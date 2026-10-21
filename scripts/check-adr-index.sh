#!/bin/sh
# Fail if a decision record is not linked from docs/decisions/index.md,
# or if two records share a number.
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

for number in $(cd "$dir" && ls [0-9][0-9][0-9][0-9]-*.md 2>/dev/null | cut -c1-4 | uniq -d); do
	echo "more than one record numbered $number" >&2
	status=1
done

exit "$status"
