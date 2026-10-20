package compose

// Detect reports whether argv is a docker compose invocation: argv[0] has base
// name docker (or docker.exe) and argv[1] is compose. Replaced in Task 8.
func Detect(argv []string) bool {
	panic("not implemented")
}

// Plan is the command line and environment to run in compose mode.
type Plan struct {
	Args    []string
	Env     []string
	Cleanup func() error
}

// Prepare builds the Plan for argv. Replaced in Task 8.
func Prepare(argv, environ []string, keysByFile map[string][]string, cwd string) (Plan, error) {
	panic("not implemented")
}
