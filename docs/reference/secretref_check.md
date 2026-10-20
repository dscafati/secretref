## secretref check

Resolve every annotated variable and report the result

### Synopsis

Check resolves every annotated variable, including those already set in the
environment, and prints one line per key: the key, the reference, and "ok" or
the failure. It never prints values. It exits 0 when every reference resolves
and 3 otherwise.

```
secretref check [flags]
```

### Options

```
      --env-file file      env file to load; repeatable; replaces .env and .env.local
  -h, --help               help for check
      --timeout duration   time limit for resolving all references (default 30s)
```

### SEE ALSO

* [secretref](secretref.md)	 - Start a command with secrets resolved from annotated .env files

