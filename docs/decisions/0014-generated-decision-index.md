# 0014. Generated decision index

Status: Accepted

Date: 2026-10-21

## Context

`index.md` lists every record with its title and status. It is the only place that shows status without opening each record, so a reader can tell a superseded record from an accepted one. `AGENTS.md` points agents to it.

The index was written by hand. Each record's title and status existed twice, once in the record and once in the index, and the check only verified that each record was linked. A record whose Status changed to `Superseded by NNNN` could stay `Accepted` in the index without any check failing.

We considered dropping the index, but a directory listing does not show status.

## Decision

`scripts/gen-adr-index.sh` builds `index.md` from each record's `# NNNN. Title` heading and `Status:` line. `make docs` runs it. Nobody edits `index.md` by hand.

`make check` fails if `index.md` differs from the generated output, or if a record lacks a heading or a Status line.

## Consequences

- The index cannot drift from the records.
- A new record or a status change needs `make docs` instead of a hand-written row.
- The introduction and the list of statuses live in the script, so changing them means editing the script.
- Two branches that add records still change the end of `index.md`, so git still reports the conflict that [0013](0013-sequential-decision-numbers.md) relies on.
