# secretref

secretref starts a command with real secrets in its environment. You mark each
secret in your `.env` file with a comment that says where the value lives.
Your app code does not change.

> Early preview. Expect breaking changes before 1.0. See [Stability](#stability).

```dotenv
# secretref: aws-sm://eu-west-1/prod/stripe#key
STRIPE_KEY=

# secretref: keyring://myapp/DB_PASSWORD
DB_PASSWORD=localdev

LOG_LEVEL=info
```

```sh
secretref run -- node server.js
```

- secretref resolves annotated variables with your own credentials. The
  resolved values replace the file's values.
- Variables without an annotation pass through unchanged.
- Variables already set in your environment win and are not resolved, so CI
  and production need no change.
- Without secretref the file is still a normal `.env` file. Dotenv loaders read
  the annotation as a comment.
- secretref never writes a resolved value to disk, stdout, stderr, or logs.
  There is no daemon and no cache.

## Install

| Method | Command |
|---|---|
| Homebrew | `brew install --cask dscafati/tap/secretref` |
| Scoop (Windows) | `scoop bucket add dscafati https://github.com/dscafati/scoop-bucket` then `scoop install dscafati/secretref` |
| winget (Windows) | `winget install dscafati.secretref` |
| npm | `npm install -g secretref` |
| Go 1.26 or later | `go install github.com/dscafati/secretref/cmd/secretref@latest` |
| deb, rpm, archives | Download from [GitHub Releases](https://github.com/dscafati/secretref/releases) |

Release binaries are built for Linux, macOS, and Windows on amd64 and arm64.
They have no runtime dependencies.

macOS binaries are not notarized. The Homebrew cask removes the quarantine
attribute when it installs. If you download an archive with a browser, run
`xattr -d com.apple.quarantine secretref` before the first start.

To check that a download is genuine, see [docs/verify.md](docs/verify.md).

## Quick start

This example uses the OS keyring, so it needs no cloud account.

1. Store a value. secretref prompts for it without echo:

   ```sh
   secretref set keyring://myapp/DB_PASSWORD
   ```

2. Annotate the variable in `.env`. The annotation applies to the line directly
   below it:

   ```dotenv
   # secretref: keyring://myapp/DB_PASSWORD
   DB_PASSWORD=localdev
   ```

3. Check that every reference resolves. `check` prints keys and references,
   never values:

   ```sh
   secretref check
   ```

4. Start your app:

   ```sh
   secretref run -- npm start
   ```

## Providers

| Scheme | Reference | Options | Credentials |
|---|---|---|---|
| `aws-sm` | `aws-sm://<region>/<name>` | `?profile=<profile>` | AWS SDK default chain |
| `gcp-sm` | `gcp-sm://<project>/<secret>` | `?version=<N>` or `?version=latest` (default `latest`) | Google Application Default Credentials |
| `keyring` | `keyring://<service>/<account>` | none | The OS keyring of the current user |

- Every scheme accepts `#<field>` at the end. secretref then parses the secret
  as a JSON object and uses the top-level string field `<field>`. Example:
  `aws-sm://eu-west-1/prod/db?profile=dev#password`.
- An `aws-sm` name may contain `/`. A `gcp-sm` secret and a `keyring` account
  may not.
- `aws-sm` reads the current version (`AWSCURRENT`) of a string secret. Binary
  secrets are not supported.

Authentication for each provider is in the [user guide](docs/guide.md#providers-and-authentication).

## Commands

| Command | What it does |
|---|---|
| `secretref run [flags] -- <command> [args...]` | Resolve and start the command. Flags: `--env-file`, `--allow-missing`, `--timeout`, `--verbose`. |
| `secretref check` | Resolve every annotated variable and print `ok` or the failure for each key. |
| `secretref set keyring://<service>/<account>` | Store a value in the OS keyring. |
| `secretref delete keyring://<service>/<account>` | Remove a value from the OS keyring. |
| `secretref version` or `secretref --version` | Print the version. |
| `secretref completion <shell>` | Print a shell completion script. |

`set` and `delete` accept only `keyring://` references. Every flag is in
[docs/reference/](docs/reference/secretref.md).

## Exit codes

| Code | Meaning |
|---|---|
| child's code | The command ran. secretref exits with its code. |
| 1 | Internal error. |
| 2 | Usage error, an invalid env file or annotation, or no env file found. |
| 3 | A reference failed to resolve. `delete` also exits 3 when the item does not exist. |
| 127 | The command was not found or is not executable. |

## Not in scope

These are decided. Changing any of them needs a new
[decision record](docs/decisions/index.md).

- Writing to cloud secret stores. `set` and `delete` work only on the local
  keyring ([0007](docs/decisions/0007-writes-only-to-local-keyring.md)).
- Writing resolved values to files or templates
  ([0002](docs/decisions/0002-resolve-at-process-start.md)).
- Refreshing values while the app runs, a daemon, or a cache. Restart the app
  to pick up a changed secret ([0002](docs/decisions/0002-resolve-at-process-start.md)).
- Overriding a variable that is already set in the environment
  ([0004](docs/decisions/0004-inject-all-variables-environment-wins.md)).
- A config file ([0003](docs/decisions/0003-annotations-are-comments.md)).
- podman-compose and `docker-compose` v1. Compose mode supports
  `docker compose` v2 only ([0009](docs/decisions/0009-compose-names-only-override.md)).
- Providers other than `aws-sm`, `gcp-sm`, and `keyring` in v0.1. HashiCorp
  Vault, Azure Key Vault, and others are not supported.
- Telemetry, usage data, crash reports, and update checks
  ([0011](docs/decisions/0011-no-telemetry.md)).

## Stability

Before 1.0, a minor release may contain breaking changes. Three things change
only with a new decision record and a callout in the release notes:

- the annotation syntax (`# secretref: <ref>`),
- the reference grammar,
- the exit codes.

## Documentation

- [User guide](docs/guide.md): annotations, file loading, expansion,
  authentication, Docker Compose, Windows, and troubleshooting.
- [Command reference](docs/reference/secretref.md), generated from the CLI.
- [Verifying a release](docs/verify.md).
- [Security policy](SECURITY.md) and threat model.
- [Contributing](CONTRIBUTING.md) and [decision records](docs/decisions/index.md).

## License

Apache-2.0. See [LICENSE](LICENSE).
