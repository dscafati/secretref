# secretref

Keep secrets out of your `.env` files. You add a comment that says where each
secret lives, and `secretref run` starts your command with the real values.
Your team's workflow, apps, and repo stay as they are. Teammates without
secretref see a normal `.env` file. Supported stores: AWS Secrets Manager,
Google Cloud Secret Manager, and the OS keyring (macOS Keychain, Windows
Credential Manager, Linux Secret Service).

Early preview. Expect breaking changes before 1.0.

```sh
npm install -g secretref
secretref run -- npm start
```

This package is a thin launcher. The native binary comes from the matching
`@secretref/<os>-<cpu>` package, installed as an optional dependency. If it is
missing, the launcher prints a hint and exits with code 1.

Documentation, other install methods, and source:
https://github.com/dscafati/secretref

- User guide: https://github.com/dscafati/secretref/blob/main/docs/guide.md
- Verifying a release: https://github.com/dscafati/secretref/blob/main/docs/verify.md
