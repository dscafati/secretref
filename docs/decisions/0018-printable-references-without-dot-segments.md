# 0018. References are printable text without dot segments

Status: Accepted

Date: 2026-10-21

## Context

The reference grammar is part of the public interface. A change to it needs a record (see the README's Stability section).

`check`, `run`, `set`, and `delete` print references on the terminal. The grammar accepted any byte, so a reference could carry an escape sequence that clears the screen, rewrites earlier lines, or sets the window title. It could also carry invisible or direction-changing characters that make one reference look like another. An env file from a pull request or a template can carry such a reference.

Providers build request paths from the location and name. A `.` or `..` segment, as in `gcp-sm://p/..`, addresses a different resource than the reference appears to name.

## Decision

`ref.Parse` rejects a reference that:

- is not valid UTF-8, or holds a character that is not printable. Control characters (below U+0020, U+007F, and U+0080 to U+009F), format characters such as U+202E, and other invisible characters are rejected. The ASCII space is printable.
- has a location or name segment that is `.` or `..`, for every scheme.
- has a query value that, once decoded, is not printable. `?profile=a%0Ab` is rejected.

Output that shows a reference uses `Ref.String()` or `ref.Error`, never the raw text. Both show text that is not printable as a quoted Go string with escapes, so a reference built without `Parse` still cannot reach the terminal unescaped.

## Consequences

- A reference that worked before can now fail with exit 2: an AWS secret name with a `.` or `..` segment, or a reference with a tab or another control character. Such names must be renamed or reached another way. The release notes call this out.
- A reference shown on the terminal is always the text the user can read, with no hidden characters.
- Names in other scripts, such as `keyring://café/ü`, still work.
