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
  # POSIX awk only: BSD sed has no \| alternation and no \n in replacements, so
  # the JSON string is decoded by a small scanner instead of a regexp.
  cmd=$(printf '%s' "$input" | awk '
    { all = all (NR > 1 ? "\n" : "") $0 }
    END {
      key = "\"command\""
      pos = 1
      while ((i = index(substr(all, pos), key)) > 0) {
        j = pos + i - 1 + length(key)
        # Skip blanks, then require a colon, blanks and the opening quote.
        while (j <= length(all) && index(" \t\r\n", substr(all, j, 1)) > 0) j++
        if (substr(all, j, 1) == ":") {
          j++
          while (j <= length(all) && index(" \t\r\n", substr(all, j, 1)) > 0) j++
          if (substr(all, j, 1) == "\"") { start = j + 1; found = 1; break }
        }
        pos = pos + i - 1 + length(key)
      }
      if (!found) exit
      out = ""
      n = length(all)
      for (k = start; k <= n; k++) {
        c = substr(all, k, 1)
        if (c == "\"") break
        if (c == "\\") {
          k++
          e = substr(all, k, 1)
          if (e == "n") c = "\n"
          else if (e == "t") c = "\t"
          else if (e == "r") c = "\r"
          else if (e == "\"" || e == "\\" || e == "/") c = e
          else c = "\\" e
        }
        out = out c
      }
      printf "%s", out
    }')
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
