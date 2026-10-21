#!/bin/sh
# Tests scripts/check-commit-msg.sh and .claude/hooks/guard-git.sh.
set -u
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
check="$root/scripts/check-commit-msg.sh"
guard="$root/.claude/hooks/guard-git.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail=0

# expect WANT NAME CMD...   (WANT is the exit code)
expect() {
  want=$1 name=$2
  shift 2
  "$@" >/dev/null 2>&1
  got=$?
  if [ "$got" != "$want" ]; then
    echo "FAIL: $name (want exit $want, got $got)" >&2
    fail=1
  fi
}

msg() { printf '%s\n' "$2" >"$tmp/$1"; }
msg ok 'feat: x

Co-Authored-By: Jane Doe <jane@example.com>'
msg claude 'feat: x

Co-Authored-By: Claude <noreply@anthropic.com>'
msg lower 'feat: x

co-authored-by: GitHub Copilot <c@example.com>'
msg eq 'feat: x

Co-authored-by=Codex <c@example.com>'
msg session 'feat: x

Claude-Session: https://claude.ai/code/session_1'
msg prose 'docs: explain that a Co-Authored-By: Claude line is rejected'

expect 0 'file: human co-author passes' sh "$check" "$tmp/ok"
expect 1 'file: claude co-author' sh "$check" "$tmp/claude"
expect 1 'file: lowercase copilot' sh "$check" "$tmp/lower"
expect 1 'file: = separator' sh "$check" "$tmp/eq"
expect 1 'file: session trailer' sh "$check" "$tmp/session"
expect 0 'file: mid-line mention passes' sh "$check" "$tmp/prose"
expect 1 'stdin: claude' sh -c "sh '$check' - <'$tmp/claude'"
expect 2 'file: missing file' sh "$check" "$tmp/nope"

cmdcheck() { printf '%s\n' "$1" | sh "$check" --command; }
expect 1 'command: -m trailer' cmdcheck 'git commit -m "x

Co-Authored-By: Claude <a@b>"'
expect 1 'command: --trailer =' cmdcheck 'git commit -m x --trailer "Co-authored-by=Claude <a@b>"'
expect 1 'command: --trailer session' cmdcheck 'git commit -m x --trailer "Claude-Session=https://x"'
expect 0 'command: plain' cmdcheck 'git commit -m "fix: y"'

# The guard reads its JSON input with jq when jq is on PATH and with sed
# otherwise. Every guard case runs through both paths. The sed path runs with a
# PATH that holds only the tools the scripts use, so jq cannot be found. A
# missing jq is reported, and fails the run in CI, so the jq path is never
# skipped silently.
nojq="$tmp/nojq-bin"
mkdir "$nojq"
for t in sh cat dirname sed grep head; do
  p=$(command -v "$t") || { echo "FAIL: $t not found" >&2; exit 1; }
  ln -s "$p" "$nojq/$t"
done
if command -v jq >/dev/null 2>&1; then
  have_jq=1
else
  have_jq=0
  echo "commit checks: jq not installed; the guard's jq path is not tested" >&2
  [ -z "${CI:-}" ] || fail=1
fi

# json COMMAND prints {"tool_input":{"command":COMMAND}} without using jq.
json() {
  esc=$(printf '%s' "$1" | awk 'BEGIN { ORS = "" } { gsub(/\\/, "\\\\"); gsub(/"/, "\\\""); if (NR > 1) print "\\n"; print }')
  printf '{"tool_input":{"command":"%s"}}' "$esc"
}

# guard NAME WANT COMMAND
guard() {
  name=$1 want=$2 input=$(json "$3")
  expect "$want" "guard (sed): $name" sh -c "printf '%s' \"\$1\" | PATH='$nojq' CLAUDE_PROJECT_DIR='$root' sh '$guard'" _ "$input"
  if [ "$have_jq" = 1 ]; then
    expect "$want" "guard (jq): $name" sh -c "printf '%s' \"\$1\" | CLAUDE_PROJECT_DIR='$root' sh '$guard'" _ "$input"
  fi
}
guard 'plain commit' 0 'git commit -m "fix: y"'
guard '--no-verify' 2 'git commit --no-verify -m x'
guard '--no-verif prefix' 2 'git commit --no-verif -m x'
guard '--no-ver prefix' 2 'git commit --no-ver -m x'
guard 'push --no-verify' 2 'git push --no-verify'
guard '-nm' 2 'git commit -nm x'
guard 'core.hooksPath' 2 'git -c core.hooksPath=/dev/null commit -m x'
guard 'LEFTHOOK=0' 2 'LEFTHOOK=0 git commit -m x'
guard 'env LEFTHOOK=0' 2 'env LEFTHOOK=0 git commit -m x'
guard 'co-author' 2 'git commit -m "x

Co-Authored-By: Claude <a@b>"'
guard 'trailer =' 2 'git commit -m x --trailer "Co-authored-by=Claude <a@b>"'
guard '-F bad file' 2 "git commit -F $tmp/claude"
guard '-F clean file' 0 "git commit -F $tmp/ok"
guard '--file= bad file' 2 "git commit --file=$tmp/session"
guard '-F unreadable' 2 "git commit -F $tmp/missing"
guard '-F stdin' 2 'git commit -F -'
guard 'unrelated command' 0 'ls -la'

[ "$fail" = 0 ] && echo "commit checks: ok"
exit "$fail"
