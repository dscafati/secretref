package envfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Load parses every file in paths and returns their entries in order. Entries
// keep the order of paths, then the order within each file. Duplicate keys are
// kept; resolving decides which one wins. Entry.File is the path as given.
// Load stops with ctx's error once ctx is done.
func Load(ctx context.Context, paths []string) ([]Entry, error) {
	var all []Entry
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := loadFile(p)
		if err != nil {
			return nil, err
		}
		all = append(all, entries...)
	}
	return all, nil
}

func loadFile(path string) ([]Entry, error) {
	f, err := os.Open(path) //nolint:gosec // The path is a user-chosen env file.
	if err != nil {
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(path, f)
}

// DefaultFiles returns the paths of .env and .env.local in dir, in that order,
// that exist as regular files.
func DefaultFiles(dir string) []string {
	var found []string
	for _, name := range []string{".env", ".env.local"} {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			found = append(found, p)
		}
	}
	return found
}
