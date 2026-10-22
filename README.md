# secretref

Keep secrets out of your `.env` files. You add a comment that says where each
secret lives, and `secretref run` starts your command with the real values.

> Early preview. Expect breaking changes before 1.0.

## Nothing to roll out

You do not change your team's workflow, your apps, or your repo. One developer
can start alone. Teammates who do not use secretref see a normal `.env` file,
because dotenv loaders read the annotation as a comment. Use it internally, on one
project at a time.

Before, `.env.local` holds a pasted secret that also gets shared over chat:

```diff
- STRIPE_KEY=sk_live_51Hxxxxxxxxxxxxxxxxxxxx
```

After, the file holds only a reference and a placeholder:

```diff
+ # secretref: aws-sm://eu-west-1/prod/stripe#key
+ STRIPE_KEY=placeholder
```

```sh
secretref run -- npm start
```

## Providers

| Store | Example reference |
|---|---|
| AWS Secrets Manager | `aws-sm://eu-west-1/prod/stripe#key` |
| GCP Secret Manager | `gcp-sm://my-project/stripe` |
| OS keyring: macOS Keychain, Windows Credential Manager, Linux Secret Service | `keyring://myapp/DB_PASSWORD` |

## Install

Pick one.

macOS or Linux, with Homebrew:

```sh
brew install --cask dscafati/tap/secretref
```

Windows, with Scoop:

```sh
scoop bucket add dscafati https://github.com/dscafati/scoop-bucket
scoop install dscafati/secretref
```

Any platform, with npm:

```sh
npm install -g secretref
```

Any platform, with Go:

```sh
go install github.com/dscafati/secretref/cmd/secretref@latest
```

## How it works

- `run` resolves the annotated keys with your own credentials when the command
  starts. It adds those keys, and any value built from them such as
  `DATABASE_URL=postgres://app:${DB_PASSWORD}@db/app`, to the command's
  environment. Other variables are left to your app's own dotenv loader.
- A variable already set in your environment wins, so CI and production need
  no change.
- Resolved values are never written to disk, stdout, stderr, or logs.
- There is no daemon, no cache, and no telemetry.

## Learn more

- [User guide](docs/guide.md)
- [Command reference](docs/reference/secretref.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)
- [Decision records](docs/decisions/index.md)
- [License](LICENSE): Apache-2.0
