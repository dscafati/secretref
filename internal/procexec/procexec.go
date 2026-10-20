package procexec

import (
	"context"
	"errors"
)

// ErrCommandNotFound reports that the command is not on the path.
var ErrCommandNotFound = errors.New("command not found")

// Replace runs argv in place of the current process on Unix; on Windows it
// spawns the child and exits with its code. Replaced in Task 9.
func Replace(argv, env []string) error {
	panic("not implemented")
}

// Spawn runs argv, forwards SIGINT and SIGTERM, and returns the child's exit
// code. Replaced in Task 9.
func Spawn(ctx context.Context, argv, env []string) (int, error) {
	panic("not implemented")
}
