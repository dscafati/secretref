---
name: write-acceptance-test
description: Use when adding or changing an end-to-end acceptance test that runs the real secretref binary through testscript.
---

# Write an acceptance test

1. Read the Testing section of `docs/style.md` and the canary rule it describes.
2. Look at the existing scripts in the acceptance test directory (see the
   `test-acceptance` target in the `Makefile` for its location) and copy the closest one.
3. Use the fake provider; never call a real cloud. Seed it with a canary value.
4. Assert the behavior (exit code, child environment). Then assert the canary is absent
   from stdout, stderr, and every file the script wrote, except the child's environment.
5. Cover Windows-relevant cases (CRLF files, arguments with spaces, exit code) when the
   change touches process start or file parsing.
6. Run `make test-acceptance`, then `make check`.
