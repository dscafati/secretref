## secretref set

Store a value in the OS keyring

### Synopsis

Set reads a value and stores it in the OS keyring under the reference. On a
terminal it prompts without echo; otherwise it reads stdin and drops one
trailing newline. Only keyring:// references are accepted.

```
secretref set keyring://<service>/<account> [flags]
```

### Examples

```
  secretref set keyring://myapp/DB_PASSWORD
  printf %s "$TOKEN" | secretref set keyring://myapp/API_TOKEN
```

### Options

```
  -h, --help   help for set
```

### SEE ALSO

* [secretref](secretref.md)	 - Start a command with secrets resolved from annotated .env files

