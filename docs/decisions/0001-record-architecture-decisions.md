# 0001. Record architecture decisions

Status: Accepted

Date: 2026-10-20

## Context

secretref is open source and will have outside contributors and AI agents. Without a written history, people reopen settled questions and change behavior that others rely on.

## Decision

We record each significant architectural decision in `docs/decisions/NNNN-title.md`, in Michael Nygard's format: Context, Decision, Consequences. Each record has a status and a date.

Accepted records are never edited, except for the Status line. To reverse a decision, write a new record that supersedes the old one, and change the old record's Status to "Superseded by NNNN". `index.md` lists every record, and CI fails if a record is missing from it.

## Consequences

- Contributors can read why the code is shaped the way it is.
- Reversing a decision costs a new record, which keeps changes deliberate.
- The pull request template asks whether a change affects a decision.
- Small decisions do not need a record. Only decisions that are hard to reverse or that surprise a reader do.
