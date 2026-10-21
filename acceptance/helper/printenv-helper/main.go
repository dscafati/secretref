package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	os.Exit(run())
}

func run() int {
	args := os.Args[1:]
	// As docker, drop the docker global flags and "compose" itself.
	for i, a := range args {
		if a == "compose" {
			args = args[i+1:]
			break
		}
	}
	fs := flag.NewFlagSet("printenv-helper", flag.ContinueOnError)
	exit := fs.Int("exit", 0, "exit with this code")
	waitSignal := fs.Bool("wait-signal", false, "write ready, wait for SIGINT or SIGTERM, record it, exit 42")
	showCompose := fs.Bool("show-compose", false, "print the last file in COMPOSE_FILE")
	writeArgs := fs.Bool("args", false, "write the arguments to child-args.txt")
	fs.Func("f", "compose file (accepted and ignored, as docker compose -f)", func(string) error { return nil })
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var lines []string
	for _, key := range fs.Args() {
		if v, ok := os.LookupEnv(key); ok {
			lines = append(lines, key+"="+v)
		} else {
			lines = append(lines, key+" unset")
		}
	}
	if err := writeLines("child-env.txt", lines); err != nil {
		return fail(err)
	}
	if *writeArgs {
		if err := writeLines("child-args.txt", fs.Args()); err != nil {
			return fail(err)
		}
	}
	if *showCompose {
		if err := printOverride(); err != nil {
			return fail(err)
		}
	}
	if *waitSignal {
		return awaitSignal()
	}
	return *exit
}

func writeLines(name string, lines []string) error {
	return os.WriteFile(name, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func printOverride() error {
	files := filepath.SplitList(os.Getenv("COMPOSE_FILE"))
	if len(files) == 0 {
		return fmt.Errorf("COMPOSE_FILE is empty")
	}
	data, err := os.ReadFile(files[len(files)-1]) //nolint:gosec // G703: test helper reads the override secretref named.
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

func awaitSignal() int {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	if err := os.WriteFile("ready", nil, 0o600); err != nil {
		return fail(err)
	}
	select {
	case s := <-sigs:
		if err := os.WriteFile("got-signal.txt", []byte(s.String()+"\n"), 0o600); err != nil {
			return fail(err)
		}
		return 42
	case <-time.After(30 * time.Second):
		return 3
	}
}

func fail(err error) int {
	_, _ = fmt.Fprintf(os.Stderr, "printenv-helper: %v\n", err)
	return 1
}
