#!/bin/sh
# Fails when a .Reveal() call in non-test Go code has no "// SECURITY:" comment
# on the same line or within the 3 preceding lines. Every place a secret becomes
# plain bytes must say why that is safe. The definition in internal/secret is
# not a call and is skipped.
set -eu
cd "$(dirname "$0")/.."
status=0
for f in $(find . -name '*.go' -not -name '*_test.go' -not -path './tools/*' -not -path './.git/*'); do
  out=$(awk '
    { line[NR] = $0 }
    /\.Reveal\(\)/ && $0 !~ /^func / {
      ok = 0
      for (i = NR - 3; i <= NR; i++) if (i >= 1 && line[i] ~ /\/\/ SECURITY:/) ok = 1
      if (!ok) printf "%s:%d: .Reveal() without a // SECURITY: comment in the 3 preceding lines\n", FILENAME, NR
    }' "$f")
  if [ -n "$out" ]; then echo "$out" >&2; status=1; fi
done
exit $status
