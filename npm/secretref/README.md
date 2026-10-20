# secretref

Resolve secrets from vaults using reference comments in your `.env` files. No code changes.

```sh
npm install -g secretref
secretref run -- npm start
```

This package is a thin launcher. The native binary comes from the matching
`@secretref/<os>-<arch>` package, installed as an optional dependency. If it is
missing, the launcher prints a hint and exits with code 1.

Documentation and source: https://github.com/dscafati/secretref
