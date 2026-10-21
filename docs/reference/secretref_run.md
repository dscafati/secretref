## secretref run

Resolve annotated variables and start a command

### Synopsis

Run loads .env then .env.local (or the --env-file list), resolves annotated
variables that are not already set in the environment, and starts the command.

By default the command gets the resolved variables and the variables whose
values are built from them with ${...}. Other variables are left to the app's
own dotenv loader. --all gives the command every variable from the files.
Variables already set in the environment always win.

On Unix secretref replaces itself with the command. On Windows, and for
"docker compose", it starts the command as a child, forwards interrupts, and
exits with the child's exit code.

```
secretref run [flags] -- <command> [args...]
```

### Examples

```
  secretref run -- npm start
  secretref run --env-file .env.staging -- ./server
  secretref run --all -- ./server
  secretref run -- docker compose up
```

### Options

```
      --all                give the command every variable from the files, not only resolved ones and those built from them
      --allow-missing      warn and keep the file's value for references that fail, instead of exiting 3
      --env-file file      env file to load; repeatable; replaces .env and .env.local
  -h, --help               help for run
      --timeout duration   time limit for resolving all references (default 30s)
      --verbose            log which key resolved from which reference (never values)
```

### SEE ALSO

* [secretref](secretref.md)	 - Start a command with secrets resolved from annotated .env files

