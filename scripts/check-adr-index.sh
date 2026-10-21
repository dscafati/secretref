#!/bin/sh
# Fail if docs/decisions/index.md differs from what gen-adr-index.sh prints,
# or if two records share a number.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
dir="$root/docs/decisions"
status=0

if ! sh "$root/scripts/gen-adr-index.sh" | diff -u "$dir/index.md" - >&2; then
	echo "docs/decisions/index.md is out of date; run make docs" >&2
	status=1
fi

for number in $(cd "$dir" && ls [0-9][0-9][0-9][0-9]-*.md 2>/dev/null | cut -c1-4 | uniq -d); do
	echo "more than one record numbered $number" >&2
	status=1
done

exit "$status"
