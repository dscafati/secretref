package envfile

import "io"

// Parse reads the env file name from r. Replaced in Task 6.
func Parse(name string, r io.Reader) ([]Entry, error) {
	panic("not implemented")
}

// Load parses every file in paths and returns their entries in order.
func Load(paths []string) ([]Entry, error) {
	panic("not implemented")
}

// DefaultFiles returns the existing files among .env and .env.local in dir.
func DefaultFiles(dir string) []string {
	panic("not implemented")
}
