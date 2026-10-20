# Security policy

## Reporting a vulnerability

Report vulnerabilities privately through a GitHub security advisory:
<https://github.com/dscafati/secretref/security/advisories/new>

Do not open a public issue for a vulnerability. We acknowledge reports within
72 hours.

## Supported versions

Only the latest minor release gets security fixes.

## Dependency vulnerabilities

A CVE in a dependency counts as a vulnerability in secretref only if
`govulncheck` reports that secretref reaches the affected code. CI runs
`govulncheck` on every pull request and on a schedule. We fix reachable
findings. We track unreachable ones and update the dependency in normal
maintenance.

## Threat model

secretref reads references from env files, resolves them with the user's own
credentials, and places the values in a child process environment. It protects
resolved values from leaking through its own output: they never reach stdout,
stderr, logs, error messages, or files. It does not protect against a hostile
child process, another process running as the same user that can read the
child's environment, a compromised secret store, or a malicious env file in the
working directory. A leak of a resolved value from secretref itself is a
vulnerability. Report it.
