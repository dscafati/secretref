#!/bin/sh
# PreToolUse hook for Bash. Exit 2 blocks the call and shows stderr to the agent.
# Blocks ways around the git hooks and AI co-author or session trailers on git
# commit. The trailer pattern lives in scripts/check-commit-msg.sh only.
set -u

root=${CLAUDE_PROJECT_DIR:-$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)}
check="$root/scripts/check-commit-msg.sh"

input=$(cat)
if command -v jq >/dev/null 2>&1; then
  cmd=$(printf '%s' "$input" | jq -r '.tool_input.command // empty')
else
  cmd=$(printf '%s' "$input" | sed -n 's/.*"command"[[:space:]]*:[[:space:]]*"\(\([^"\\]\|\\.\)*\)".*/\1/p' | sed 's/\\n/\n/g; s/\\"/"/g')
fi

block() {
  echo "Blocked: $1" >&2
  exit 2
}

# Intentionally broad: these are blocked anywhere in the command, even inside a
# quoted message, because false positives are cheap and bypasses are not.
# git accepts any unambiguous prefix of --no-verify, so match from --no-ve.
case "$cmd" in
*--no-ve*) block "--no-verify (or an abbreviation) skips the git hooks. Fix the failing check instead." ;;
esac
if printf '%s\n' "$cmd" | grep -Eiq 'core\.hookspath|--config-env'; then
  block "overriding core.hooksPath or config from the environment disables the git hooks."
fi
if printf '%s\n' "$cmd" | grep -Eq '(^|[^A-Za-z0-9_])LEFTHOOK(_EXCLUDE)?='; then
  block "LEFTHOOK and LEFTHOOK_EXCLUDE turn the git hooks off. Fix the failing check instead."
fi

# -n is the short form of --no-verify on git commit, alone or combined (-nm, -anm).
# Also intentionally broad: any standalone short-option word containing n after
# `commit` in the same command segment is blocked, including inside message text.
if printf '%s\n' "$cmd" | grep -Eq '(^|[;&|[:space:]])git[[:space:]]([^;&|]*[[:space:]])?commit([[:space:]][^;&|]*)?[[:space:]]-[a-zA-Z]*n[a-zA-Z]*([[:space:]]|$)'; then
  block "git commit -n is --no-verify. Fix the failing check instead."
fi

case "$cmd" in
*git*commit*)
  printf '%s\n' "$cmd" | sh "$check" --command || exit 2
  # -F/--file reads the message from a file the command line does not show.
  file=$(printf '%s\n' "$cmd" | sed -n -E "s/.*[[:space:]](-F|--file)[=[:space:]]*['\"]?([^'\"[:space:];&|]+).*/\2/p" | head -n 1)
  if printf '%s\n' "$cmd" | grep -Eq '[[:space:]](-F|--file)([=[:space:]]|$)|[[:space:]]-[a-zA-Z]*F'; then
    if [ -z "$file" ] || [ "$file" = "-" ] || [ ! -r "$file" ]; then
      block "git commit -F/--file: the message file cannot be checked. Use -m, or a readable file."
    fi
    sh "$check" "$file" || exit 2
  fi
  ;;
esac
exit 0
