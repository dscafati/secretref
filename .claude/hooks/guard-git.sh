#!/bin/sh
# PreToolUse hook for Bash. Exit 2 blocks the call and shows stderr to the agent.
# Blocks --no-verify and AI co-author or session trailers on git commit.
# The regexes match the commit-msg check in CI and docs/decisions/0012.
set -u

input=$(cat)
if command -v jq >/dev/null 2>&1; then
  cmd=$(printf '%s' "$input" | jq -r '.tool_input.command // empty')
else
  cmd=$(printf '%s' "$input" | sed -n 's/.*"command"[[:space:]]*:[[:space:]]*"\(\([^"\\]\|\\.\)*\)".*/\1/p' | sed 's/\\n/\n/g; s/\\"/"/g')
fi

# Intentionally broad: --no-verify anywhere in the command is blocked, even
# inside a quoted message, because false positives are cheap and bypasses are not.
case "$cmd" in
*--no-verify*)
  echo "Blocked: --no-verify skips the git hooks. Fix the failing check instead." >&2
  exit 2
  ;;
esac

# -n is the short form of --no-verify on git commit, alone or combined (-nm, -anm).
# Also intentionally broad: any standalone short-option word containing n after
# `commit` in the same command segment is blocked, including inside message text.
if printf '%s\n' "$cmd" | grep -Eq '(^|[;&|[:space:]])git[[:space:]]([^;&|]*[[:space:]])?commit([[:space:]][^;&|]*)?[[:space:]]-[a-zA-Z]*n[a-zA-Z]*([[:space:]]|$)'; then
  echo "Blocked: git commit -n is --no-verify. Fix the failing check instead." >&2
  exit 2
fi

case "$cmd" in
*git*commit*)
  if printf '%s\n' "$cmd" | grep -Eiq 'co-authored-by:.*(claude|anthropic|copilot|openai|chatgpt|gemini|cursor|codex)|claude-session:'; then
    echo "Blocked: AI co-author and Claude-Session trailers are not allowed (docs/decisions/0012). Remove them." >&2
    exit 2
  fi
  ;;
esac
exit 0
