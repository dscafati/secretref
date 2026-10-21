#!/bin/sh
# Rejects AI co-author and session trailers (docs/decisions/0012). This file holds
# the only copy of the trailer pattern. It is used by the lefthook commit-msg
# hook, .claude/hooks/guard-git.sh, and the commit-trailers job in CI.
#
# Usage:
#   check-commit-msg.sh FILE        check a commit message file ("-" reads stdin)
#   check-commit-msg.sh --command   read a shell command on stdin and check it for
#                                   trailers passed inline (-m, --trailer)
#
# File mode matches only at the start of a line, as git reads trailers. Command
# mode matches anywhere, because the trailer sits inside a quoted argument. Both
# accept ":" and "=" (git commit --trailer "Co-authored-by=..."), any case, and
# optional whitespace before the separator.
set -u

ai='claude|anthropic|copilot|openai|chatgpt|gemini|cursor|codex'
key="(co-authored-by[[:space:]]*[:=].*($ai)|claude-session[[:space:]]*[:=])"

mode=file
case "${1:-}" in
--command)
  mode=command
  pattern=$key
  ;;
"")
  echo "usage: check-commit-msg.sh FILE|-  or  check-commit-msg.sh --command" >&2
  exit 2
  ;;
*) pattern="^[[:space:]]*$key" ;;
esac

if [ "$mode" = command ] || [ "$1" = "-" ]; then
  text=$(cat)
else
  [ -r "$1" ] || { echo "check-commit-msg: cannot read $1" >&2; exit 2; }
  text=$(cat "$1")
fi

if printf '%s\n' "$text" | grep -Eiq "$pattern"; then
  echo "Blocked: AI co-author and Claude-Session trailers are not allowed (docs/decisions/0012). Remove them." >&2
  exit 1
fi
exit 0
