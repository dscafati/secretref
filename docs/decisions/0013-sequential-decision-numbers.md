# 0013. Sequential decision numbers

Status: Accepted

Date: 2026-10-21

## Context

Each record takes the next free number. Two branches written in parallel can both take the same number. We considered four ways to handle this:

- Keep sequential numbers. The branch that merges second renumbers its record.
- Use a placeholder `NNNN` while drafting and assign the number with a script just before merge.
- Use the pull request number. This ties records to GitHub and leaves gaps in the sequence.
- Allow duplicate numbers and cite records only by filename. The number then only sorts records, and existing citations such as "ADR 0002" become ambiguous.

Records are rare and mostly written by one person, so collisions will be rare. When one happens, both branches append a row to the end of `index.md`, and git reports a conflict.

## Decision

Records keep unique, sequential, four-digit numbers. A new record takes the next free number on its branch. If another record with that number merges first, the second branch renames its file, its heading, and its `index.md` row to the next free number before merging.

`scripts/check-adr-index.sh` fails if two records share a number, so `make check` and CI catch a duplicate that slips past the merge.

## Consequences

- Numbers stay short, dense, and ordered, and a bare number like "0002" is enough to cite a record.
- Renumbering is safe before merge, because nothing can cite an unmerged record yet.
- The author of the second branch does the rename by hand. No tooling assigns numbers.
- If parallel records become common, a later record can switch to assigning numbers at merge time.
