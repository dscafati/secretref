## secretref

Start a command with secrets resolved from annotated .env files

### Synopsis

secretref reads .env and .env.local, resolves the variables annotated with
a "# secretref: <ref>" comment, and starts a command with the real values in
its environment. Values are never written to disk, stdout, or logs.

```
secretref [flags]
```

### Options

```
  -h, --help   help for secretref
```

### SEE ALSO

* [secretref check](secretref_check.md)	 - Resolve every annotated variable and report the result
* [secretref completion](secretref_completion.md)	 - Generate the autocompletion script for the specified shell
* [secretref delete](secretref_delete.md)	 - Remove a value from the OS keyring
* [secretref run](secretref_run.md)	 - Resolve annotated variables and start a command
* [secretref set](secretref_set.md)	 - Store a value in the OS keyring
* [secretref version](secretref_version.md)	 - Print the secretref version

