#!/bin/sh
# Fails if docs/decisions/index.md differs from what gen-adr-index.sh prints, a
# link in the index does not resolve, two records share a number, or a record
# that exists on the base was edited other than its Status line.
#
# The base is $ADR_BASE if set (CI passes the pull request base or the push's
# previous tip; it needs full history), else the merge-base of HEAD with
# origin/main or main, else HEAD~1. With no base, the immutability check is
# skipped with a notice. The comparison includes uncommitted edits.
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

# Every link in the index points at a record that exists.
for target in $(sed -n 's/.*](\([^)]*\)).*/\1/p' "$dir/index.md"); do
	if [ ! -f "$dir/$target" ]; then
		echo "docs/decisions/index.md links to $target, which does not exist" >&2
		status=1
	fi
done

# Accepted records are not edited, except for the Status line.
base=${ADR_BASE:-}
if [ -z "$base" ] || ! git -C "$root" rev-parse --verify -q "$base^{commit}" >/dev/null 2>&1; then
	base=
	for ref in origin/main main; do
		if git -C "$root" rev-parse --verify -q "$ref^{commit}" >/dev/null 2>&1; then
			base=$(git -C "$root" merge-base HEAD "$ref" 2>/dev/null || true)
			[ -n "$base" ] && break
		fi
	done
	if [ -z "$base" ]; then
		base=$(git -C "$root" rev-parse -q --verify HEAD~1 2>/dev/null || true)
	fi
fi
if [ -z "$base" ]; then
	echo "adr-check: no base commit found, skipping the immutability check" >&2
else
	changes=$(git -C "$root" diff --name-status "$base" -- 'docs/decisions/[0-9][0-9][0-9][0-9]-*.md')
	printf '%s\n' "$changes" | while IFS="$(printf '\t')" read -r kind path _; do
		[ -n "$kind" ] || continue
		case "$kind" in
		A*) ;;
		M)
			edits=$(git -C "$root" diff -U0 "$base" -- "$path" | grep -E '^[-+]' | grep -Ev '^(\+\+\+|---) ' | grep -Ev '^[-+]Status: ' || true)
			if [ -n "$edits" ]; then
				echo "$path: accepted records change only their Status line; write a superseding record instead" >&2
				exit 1
			fi
			;;
		*)
			echo "$path: records are not deleted or renamed ($kind)" >&2
			exit 1
			;;
		esac
	done || status=1
fi

exit "$status"
