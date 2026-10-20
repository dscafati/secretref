## secretref completion fish

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	secretref completion fish | source

To load completions for every new session, execute once:

	secretref completion fish > ~/.config/fish/completions/secretref.fish

You will need to start a new shell for this setup to take effect.


```
secretref completion fish [flags]
```

### Options

```
  -h, --help              help for fish
      --no-descriptions   disable completion descriptions
```

### SEE ALSO

* [secretref completion](secretref_completion.md)	 - Generate the autocompletion script for the specified shell

