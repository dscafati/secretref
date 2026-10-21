# User guide

This guide explains how secretref reads env files, resolves references, and
starts your command. For a short overview, see the [README](../README.md). For
every flag, see the [command reference](reference/secretref.md).

- [Annotations](#annotations)
- [Which files load](#which-files-load)
- [Env file syntax](#env-file-syntax)
- [Variable expansion](#variable-expansion)
- [Dotenv loaders that override](#dotenv-loaders-that-override)
- [Providers and authentication](#providers-and-authentication)
- [JSON fields](#json-fields)
- [Running a command](#running-a-command)
- [Checking references](#checking-references)
- [Storing values in the keyring](#storing-values-in-the-keyring)
- [Docker Compose](#docker-compose)
- [Windows](#windows)
- [Troubleshooting](#troubleshooting)

## Annotations

An annotation is a comment of the form `# secretref: <ref>`. It applies to the
`KEY=` line directly below it.

```dotenv
# secretref: gcp-sm://my-project/api-key
API_KEY=dev-key
```

- The annotation must be on its own line. The space after `#` and after
  `secretref:` is optional.
- It holds exactly one reference and nothing else. A trailing comment on the
  same line is an error.
- The variable must be on the next line. A blank line or another comment in
  between is an error.
- An annotation with no variable directly below it is an error. secretref
  names the file and line and exits 2.
- A reference that does not parse is an error with the file and line, exit 2.
  The message does not repeat the annotation text, in case a secret was pasted
  there by mistake.
- The value after `=` is the fallback for people who run the app without
  secretref, and for `--allow-missing`. It may be empty.

Any comment that starts with `# secretref:` is read as an annotation. Do not
use that prefix for ordinary comments.

The design is in [decision 0003](decisions/0003-annotations-are-comments.md).

## Which files load

By default, secretref loads `.env` and then `.env.local` from the current
directory. Either file may be missing. If both are missing, secretref exits 2.

`--env-file <file>` replaces the defaults. Repeat it to load several files, in
the order given. Relative paths are relative to the current directory. A
missing file is an error, exit 2.

```sh
secretref run --env-file .env --env-file .env.staging -- ./server
```

When a key appears more than once, the last definition wins, including its
annotation. If `.env` annotates `DB_PASSWORD` and `.env.local` sets
`DB_PASSWORD=x` without an annotation, the child gets `x` and nothing is
resolved.

### The existing environment wins

A variable that is already set in secretref's own environment keeps its value.
secretref does not override it and does not resolve its reference. This keeps
CI and production unchanged: they already set real values.

To use the secret from the file instead, unset the variable first:

```sh
env -u DB_PASSWORD secretref run -- npm start
```

`run` gives the child the whole existing environment plus every variable from
the files, annotated or not. See
[decision 0004](decisions/0004-inject-all-variables-environment-wins.md).

## Env file syntax

- `KEY=value` lines, with an optional `export ` prefix.
- A key starts with a letter or `_` and continues with letters, digits, `_`, or
  `.`.
- Unquoted values are trimmed. A `#` with a space or tab before it starts a
  comment. A `#` with no space before it is part of the value.
- Single-quoted values are literal. No escapes are processed, and no variables
  are expanded.
- Double-quoted values process `\n`, `\r`, `\t`, `\"`, `\\`, and `\$`.
- Quoted values may span several lines.
- Backtick quotes are not supported.
- LF and CRLF line endings both work.

Quote any value that contains `#`, so every dotenv loader reads it the same
way.

## Variable expansion

Unquoted and double-quoted values expand variables. Single-quoted values do
not.

| Form | Result |
|---|---|
| `${NAME}` or `$NAME` | The value of `NAME`, or the empty string if it is not defined. |
| `${NAME:-default}` | The value of `NAME`, or `default` if `NAME` is unset or empty. |
| `\$` | A literal `$`. |

- secretref looks a name up in the final file variables first, after the
  precedence rules above, then in the process environment. `${HOME}` works.
- The order of lines does not matter. A value may refer to a variable defined
  later in the file.
- The default in `${NAME:-default}` is literal text. `${A:-$B}` gives the two
  characters `$B` when `A` is unset.
- `$` followed by anything other than a name stays as written. So does `${`
  with no closing `}`.
- If variables refer to each other in a cycle, the reference that closes the
  cycle expands to the empty string. It is not an error.

Resolved secrets are substituted into other values, so you can build a
composite value:

```dotenv
# secretref: keyring://myapp/DB_PASSWORD
DB_PASSWORD=localdev
DATABASE_URL=postgres://app:${DB_PASSWORD}@localhost:5432/app
```

The child gets `DATABASE_URL` with the real password in it.

A resolved secret is never expanded itself. A secret whose text contains
`${X}` reaches the child exactly as stored. Values from the process
environment are not expanded either.

## Dotenv loaders that override

secretref puts the resolved values in the environment, then your app's dotenv
loader reads `.env` again. Most loaders keep variables that are already set,
so the real values survive.

Some loaders can replace existing variables. With these, the placeholder from
the file replaces the resolved value:

- Node `dotenv` with `override: true`
- Ruby `Dotenv.overload`
- Go `godotenv.Overload`
- Python `python-dotenv` with `override=True`

Use the default, non-overriding mode of your loader when you run under
secretref.

## Providers and authentication

Providers read secretref's own environment and the user's credential files.
Variables in the env file do not configure providers. Setting `AWS_PROFILE` in
`.env` does not change which profile secretref uses.

### aws-sm: AWS Secrets Manager

Reference: `aws-sm://<region>/<name>[?profile=<profile>][#<field>]`

```dotenv
# secretref: aws-sm://eu-west-1/prod/stripe?profile=prod#key
STRIPE_KEY=
```

- The region comes from the reference. `AWS_REGION` does not change it.
- The name may contain `/`, as in `prod/stripe`. It must not have an empty
  segment.
- secretref reads the `AWSCURRENT` version. Binary secrets are not supported.
- Credentials come from the AWS SDK for Go default credential chain: the
  `AWS_ACCESS_KEY_ID` family of variables, `AWS_PROFILE`, the shared config and
  credentials files, SSO, and container or instance roles.
- `?profile=<profile>` selects a profile from the shared config for that
  reference. A profile that does not exist fails as `invalid`.
- For SSO profiles, sign in first:

  ```sh
  aws sso login --profile prod
  ```

- The credentials need `secretsmanager:GetSecretValue` on the secret.

### gcp-sm: Google Cloud Secret Manager

Reference: `gcp-sm://<project>/<secret>[?version=<N>|latest][#<field>]`

```dotenv
# secretref: gcp-sm://my-project/api-key?version=3
API_KEY=
```

- Without `?version`, secretref reads `latest`. A version is a positive integer
  with no sign or leading zeros, or `latest`.
- The secret name must not contain `/`.
- Credentials come from Google Application Default Credentials:
  `GOOGLE_APPLICATION_CREDENTIALS`, the file that gcloud writes, or the
  metadata server on Google Cloud. On a developer machine, run:

  ```sh
  gcloud auth application-default login
  ```

- The credentials need permission to access secret versions, for example the
  `roles/secretmanager.secretAccessor` role.

### keyring: the OS keyring

Reference: `keyring://<service>/<account>[#<field>]`

```dotenv
# secretref: keyring://myapp/DB_PASSWORD
DB_PASSWORD=localdev
```

- The reference maps to the keyring entry with that service and account (user).
  The account must not contain `/`.
- Backends:
  - macOS: the Keychain, through the `security` tool.
  - Windows: Credential Manager.
  - Linux and other Unix systems: the Secret Service API over D-Bus. You need a
    running provider such as GNOME Keyring or KeePassXC. Headless machines
    often have none, and lookups then fail as `unavailable`.
- Entries belong to the current OS user.
- secretref reads items written by `secretref set`, which uses go-keyring's
  encoding. On macOS, items created by other tools (Keychain Access, the
  `security` tool) may come back trimmed or hex-encoded, because go-keyring
  decodes what it reads. Store values with `secretref set`.
- Size limits: Windows Credential Manager takes about 2560 bytes and the macOS
  `security` command about 3000 bytes of value. `secretref set` rejects a larger
  value as `invalid`. Store large secrets in aws-sm or gcp-sm.

## JSON fields

Every scheme accepts `#<field>` at the end of the reference. secretref parses
the secret as a JSON object and uses the top-level field `<field>`, which must
be a string.

```dotenv
# secretref: aws-sm://eu-west-1/prod/db#password
DB_PASSWORD=
```

Put options before the field: `aws-sm://eu-west-1/prod/db?profile=dev#password`.
A secret that is not a JSON object, a missing field, or a field that is not a
string fails as `invalid`. Nested fields are not supported.

## Running a command

```sh
secretref run [flags] -- <command> [args...]
```

The `--` is required. secretref runs the command directly, without a shell. To
use pipes or shell syntax, start a shell yourself:

```sh
secretref run -- sh -c 'npm run build && npm start'
```

Your interactive shell expands `$VAR` in the command line before secretref
starts, so `secretref run -- echo $DB_PASSWORD` does not print the resolved
value. `secretref run -- sh -c 'echo $DB_PASSWORD'` does. Do not do that in a
shared terminal or a CI log.

A bare command name is looked up in the `PATH` the child will get. Empty and
relative `PATH` entries are skipped, so a bare name never runs a file from the
current directory. Use `./app` for that.

On Linux and macOS, secretref replaces itself with the command (`exec`). No
secretref process stays running, and signals go straight to your command. In
compose mode and on Windows, secretref starts the command as a child and exits
with its exit code. A child killed by a signal gives 128 plus the signal
number.

### Flags

- `--env-file <file>`: load this file instead of `.env` and `.env.local`.
  Repeatable.
- `--allow-missing`: when a reference fails for any reason, use the value from
  the file and print a warning on stderr. Without it, any failure stops
  secretref with exit 3 and the command does not start.
- `--timeout <duration>`: the time limit for resolving all references
  together, for example `10s` or `2m`. The default is `30s`. References still
  pending at the limit fail as `unavailable`.
- `--verbose`: log on stderr which key resolved from which reference. Values
  are never logged.

secretref resolves up to 8 references at a time.

## Checking references

```sh
secretref check [--env-file <file>] [--timeout <duration>]
```

`check` resolves every annotated variable and prints a table of the key, the
reference, and `ok` or the failure. It never prints values. It exits 0 when
every reference resolves and 3 otherwise.

Unlike `run`, `check` also resolves keys that are already set in the
environment. Use it to test your credentials and references before a run.

## Storing values in the keyring

```sh
secretref set keyring://myapp/DB_PASSWORD
secretref delete keyring://myapp/DB_PASSWORD
```

- `set` and `delete` accept only `keyring://` references. Any other scheme is
  a usage error, exit 2. Create cloud secrets with your cloud's own tools. See
  [decision 0007](decisions/0007-writes-only-to-local-keyring.md).
- On a terminal, `set` prompts for the value without echo.
- When stdin is not a terminal, `set` reads all of stdin and removes one
  trailing newline (`\n` or `\r\n`). Other whitespace is kept.

  ```sh
  printf %s "$TOKEN" | secretref set keyring://myapp/API_TOKEN
  ```

- An empty value is a usage error, exit 2. Nothing is stored.
- `delete` exits 3 if the entry does not exist.
- Both commands give up after 30 seconds if the keyring does not answer.

## Docker Compose

Compose reads `env_file:` itself, so the environment that secretref sets does
not reach the containers on its own. secretref handles this when the command is
`docker compose` (Compose v2):

```sh
secretref run -- docker compose up
```

1. secretref finds the compose files the way Compose does: the `-f` flags,
   else `COMPOSE_FILE`, else the default file names (`compose.yaml`,
   `compose.yml`, `docker-compose.yaml`, `docker-compose.yml`) in the current
   directory or a parent directory. secretref reads `COMPOSE_FILE` and
   `COMPOSE_PATH_SEPARATOR` only from the environment it started in, never
   from an env file, because an env-file value can be built from a secret.
2. For each service, it picks the keys that secretref resolved from an
   annotation, when the file that gave the key its final value is one the
   service lists in `env_file:`. It leaves out keys that the service sets in
   its own `environment:`, in any compose file. Compose reads every other key
   from `env_file:` itself, as it would without secretref.
3. It writes a temporary override file with mode 0600. The file holds names
   only, in the form `KEY: ${KEY}`. It never holds a value.
4. It starts `docker compose` with the override added. Compose fills in
   `${KEY}` from the environment that secretref set. `environment:` takes
   precedence over `env_file:`, so the containers get the resolved values.
5. It stays running while Compose runs and passes interrupt and termination
   signals on to it. After Compose exits, it removes the override and exits
   with Compose's exit code.

If no service uses an env file with a resolved key, secretref writes no
override and prints a warning. The containers then see the file values.

Limits:

- Only `docker compose` is detected. `docker-compose` v1 and podman-compose
  run as ordinary commands, and their containers see the file values.
- If a docker global flag that secretref does not know comes before `compose`,
  secretref runs the command as an ordinary command.
- Variables that were already set in the environment are not added to the
  override. Compose reads those from `env_file:` as usual.
- Compose substitutes `${NAME}` only for names made of letters, digits, and
  `_` that do not start with a digit. A resolved key with another name, such
  as `app.port`, is left out of the override with a warning, and the
  containers see its value from `env_file:`.
- `COMPOSE_FILE` set in an env file does not choose the compose files. secretref
  prints a warning; set it in your shell or pass `-f` instead.

`docker compose config` prints the resolved values. Its stdout belongs to
Compose, not to secretref. See [SECURITY.md](../SECURITY.md).

The design is in [decision 0009](decisions/0009-compose-names-only-override.md).

## Windows

- Windows cannot replace a process, so secretref starts the command as a
  child, waits for it, and exits with its exit code.
- On Unix in compose mode, secretref does not forward a SIGINT that is sent
  directly to its own PID while it is in the terminal's foreground process
  group, because it cannot tell that signal from the terminal's own Ctrl+C,
  which already reaches the child. Send such signals to the process group, or
  use SIGTERM.
- On Ctrl+C, the console sends the interrupt to both secretref and the child.
  secretref ignores it and waits for the child to exit. The child decides how
  to stop.
- A command name that contains `\`, `/`, or `:` is used as given. A bare name
  is searched in `PATH`. Unlike `cmd.exe`, secretref does not search the
  current directory. Use `.\app.exe` for a program there.
- CRLF env files work.
- `.cmd` and `.bat` shims (for example `npm.cmd`) run through `cmd.exe`, which
  interprets `&`, `|`, `%`, and `^` in arguments. Invoke `node` or the real
  `.exe` directly when an argument can contain those characters.

## Troubleshooting

Diagnostics go to stderr. Stdout belongs to your command.

| Exit code | Meaning | What to check |
|---|---|---|
| child's code | Your command ran and exited with this code. | Your command's own output. |
| 1 | Internal error. | The error text. Report a bug if it persists. |
| 2 | Usage error, a bad env file or annotation, or no env file. | The `file:line` in the message, the reference syntax and scheme, the `--` before the command, and that `.env` or `.env.local` exists in the current directory. |
| 3 | A reference failed to resolve. | The failure kind below. Run `secretref check` for the full list. |
| 126 | The command exists but cannot be run. | The file mode (`chmod +x`), that it is not a directory, and a `#!` line for scripts. |
| 127 | Command not found. | The command name and the `PATH` the child gets. |

When a reference fails, secretref prints one line per failed key with the
reference and a failure kind:

- `not found`: the secret does not exist. Check the region or project, the
  name, and the version.
- `denied`: no credentials, or not allowed. For AWS, set `AWS_PROFILE`, add
  `?profile=`, or run `aws sso login`. For GCP, run
  `gcloud auth application-default login`. Check the IAM permissions above.
- `unavailable`: the store could not be reached in time. Check the network,
  raise `--timeout`, or on Linux check that a Secret Service provider is
  running.
- `invalid`: the request or the secret is not usable. Examples: a profile
  that does not exist, an AWS binary secret, or a `#field` that is missing or
  not a string.

A reference that does not parse, such as one with an unknown scheme, is caught
when the file loads. It is a usage error, exit 2, not a resolution failure.

`secretref run --verbose` shows which keys resolved from which references.
