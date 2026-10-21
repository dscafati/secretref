package cli

import (
	"errors"
	"fmt"
	"io"
)

// Exit codes. A command that ran returns its own code instead.
const (
	exitOK       = 0
	exitInternal = 1
	exitUsage    = 2
	exitResolve  = 3
	exitNotExec  = 126
	exitNotFound = 127
)

// exitError ends a command with code. An empty msg means the command already
// reported the problem on stderr.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func usageError(format string, args ...any) error {
	return &exitError{code: exitUsage, msg: fmt.Sprintf(format, args...)}
}

func internalError(err error) error {
	return &exitError{code: exitInternal, msg: err.Error()}
}

// exitWith ends a command with code without printing anything more.
func exitWith(code int) error {
	return &exitError{code: code}
}

// asUsage tags an error from cobra's argument or flag checks as a usage error.
func asUsage(err error) error {
	var ee *exitError
	if errors.As(err, &ee) {
		return err
	}
	return &exitError{code: exitUsage, msg: err.Error()}
}

// exitCode reports err on stderr and returns the process exit code. Usage
// errors are tagged where they arise, so an untagged error is unexpected and
// counts as internal.
func exitCode(err error, stderr io.Writer) int {
	if err == nil {
		return exitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if ee.msg != "" {
			_, _ = fmt.Fprintf(stderr, "secretref: %s\n", ee.msg)
		}
		return ee.code
	}
	_, _ = fmt.Fprintf(stderr, "secretref: %v\n", err)
	return exitInternal
}
