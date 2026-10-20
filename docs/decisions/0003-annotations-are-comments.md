# 0003. Annotations are comments; files stay valid dotenv

Status: Accepted

Date: 2026-10-20

## Context

Teams already have `.env` and `.env.local` files used by apps, scripts, and Docker Compose. A new file format or a config file would force every teammate and every tool to change. People who do not use secretref must keep working.

## Decision

A reference is a comment that matches `#\s*secretref:\s*<ref>`. It applies to the `KEY=` line directly below it. The value after `=` stays as a fallback for people who do not use secretref.

An annotation with no key directly below it is an error that names the file and line. secretref has no config file.

## Consequences

- Every file stays valid dotenv. Standard loaders ignore the annotation as a comment.
- Adoption needs one added comment line per secret. Nothing else changes.
- The annotation syntax is part of the public interface. Changing it needs a new record.
- Annotations cannot carry structured options beyond what the reference URL allows.
