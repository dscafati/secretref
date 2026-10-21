#!/bin/sh
# Fails unless .claude/skills points at ../.agents/skills.
#
# A symlink is the normal form. A checkout with core.symlinks=false (the Windows
# default) writes the link as a plain file whose whole content is the target,
# so that form is accepted too. Anything else fails.
set -eu
cd "$(dirname "$0")/.."
link=.claude/skills
target=../.agents/skills
if [ -L "$link" ]; then
  [ "$(readlink "$link")" = "$target" ] && exit 0
elif [ -f "$link" ]; then
  # Exact match: no trailing newline, no CR, nothing else in the file.
  [ "$(cat "$link"; printf x)" = "${target}x" ] && exit 0
fi
echo "$link must be a symlink to $target" >&2
exit 1
