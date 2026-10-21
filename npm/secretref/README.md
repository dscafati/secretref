# secretref

secretref starts a command with real secrets in its environment. You mark each
secret in your `.env` file with a comment that says where the value lives.
Your app code does not change. Supported stores: AWS Secrets Manager
(`aws-sm`), Google Cloud Secret Manager (`gcp-sm`), and the OS keyring
(`keyring`).

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
