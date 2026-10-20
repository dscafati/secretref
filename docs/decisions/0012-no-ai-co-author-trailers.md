# 0012. No AI co-author trailers

Status: Accepted

Date: 2026-10-20

## Context

AI tools are welcome in this project. Some add `Co-Authored-By` or session trailers to commits automatically. A person owns every change and answers for it in review. Trailers that credit a tool blur that ownership and clutter history.

## Decision

Commits must not contain `Co-Authored-By` trailers naming AI tools, or `Claude-Session` trailers. CI rejects them with these patterns:

- `(?i)^co-authored-by:.*(claude|anthropic|copilot|openai|chatgpt|gemini|cursor|codex)`
- `(?i)^claude-session:`

Contributors disclose AI use in the Authorship section of the pull request instead. `AI_POLICY.md` describes the rules. A repository hook blocks these trailers locally for agents.

## Consequences

- History shows human authors and responsible owners.
- Disclosure lives in the pull request, where reviewers read it.
- A contributor who forgets must amend the commit before merge.
- Real human co-authors are still allowed.
