# secretref

Resolve secrets from vaults using reference comments in your `.env` files.
Your app code does not change.

> Early preview. The resolver is in active development.

## How it works

Annotate a variable with a comment that says where its real value lives:

```dotenv
# secretref: vault://kv/app/db#url
DATABASE_URL=postgres://localhost:5432/dev

# secretref: aws-sm://eu-west-1/prod/stripe#key
STRIPE_KEY=

LOG_LEVEL=info
```

Then start your app through `secretref`:

```sh
secretref run -- node server.js
```

- Annotated variables are read from the vault with your own credentials and
  override the value in the file.
- Variables without an annotation pass through unchanged.
- Without `secretref`, the file is still a normal `.env` file: every dotenv
  loader ignores the comments.
- Resolved values are never written back to disk.

## Planned providers

- HashiCorp Vault (`vault://`)
- AWS Secrets Manager (`aws-sm://`)
- Google Cloud Secret Manager (`gcp-sm://`)
- Azure Key Vault (`azure-kv://`)

## License

Apache-2.0
