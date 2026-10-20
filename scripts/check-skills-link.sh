#!/bin/sh
# Fails unless .claude/skills is a symlink to ../.agents/skills.
set -eu
cd "$(dirname "$0")/.."
if [ ! -L .claude/skills ] || [ "$(readlink .claude/skills)" != "../.agents/skills" ]; then
  echo ".claude/skills must be a symlink to ../.agents/skills" >&2
  exit 1
fi
