---
name: pre-pr-review
description: Use before opening a pull request. Dispatches a fresh reviewer with no authoring context to review the diff against the project rules; reports findings and edits nothing.
---

# Pre-PR review

Review by someone who did not write the change catches more than self-review.

1. Run `make check`. Do not proceed on failure.
2. Produce the diff against the base branch (`git diff main...HEAD`).
3. Dispatch a fresh reviewer (a new subagent or session). Give it only:
   - the diff,
   - `AGENTS.md`,
   - `docs/style.md`,
   - `.github/review-rules.md`,
   - the records in `docs/decisions/`.
   Do not give it your reasoning or the task description.
4. Instruct the reviewer to report findings only, each with file, line, the rule it
   breaks, and severity. It must not edit files.
5. Triage the findings yourself. Fix real ones, add a failing-first test for bugs, and
   rerun `make check`. If a finding shows a rule the tooling should have caught, propose a
   lint, hook, or doc change.
6. Summarize what you fixed and what you dismissed, with reasons, in the PR description.
