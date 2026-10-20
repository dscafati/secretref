// Package envfile parses .env files into ordered entries, including the
// reference annotations that secretref resolves.
//
// Syntax: KEY=VALUE lines with an optional "export " prefix. Values may be
// unquoted, single-quoted (literal, may span lines), or double-quoted (escapes
// \n \r \t \" \\ \$, may span lines). Backtick quotes are not supported.
//
// In an unquoted value, " #" or a tab then "#" starts a comment, but a "#" with
// no whitespace before it is kept (python-dotenv and Docker Compose behavior;
// Node dotenv strips it). Quote values that contain "#" to behave the same
// everywhere.
//
// An annotation comment "# secretref: <ref>" applies to the KEY line directly
// below it. Both LF and CRLF line endings are accepted.
package envfile
