//go:build acceptance

// Package acceptance runs the real secretref binary, built with the
// acceptance tag so it uses the fake provider, through testscript.
package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
)

// canary is the value of every secret the fake provider holds. It may appear
// only in the child's environment, which the helper writes to child-env.txt.
const canary = "SR_CANARY_7f3a9c"

// binEnv passes the directory of the built binaries from TestMain to Setup.
const binEnv = "SECRETREF_ACCEPTANCE_BIN"

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	dir, err := os.MkdirTemp("", "secretref-acceptance-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	if err := buildAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := writeFakeSecrets(filepath.Join(dir, "fake.json")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	os.Setenv(binEnv, dir)
	return m.Run()
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// buildAll builds secretref with the acceptance tag, the helper, and the
// helper again under the name docker for compose mode.
func buildAll(dir string) error {
	builds := []struct {
		out, pkg string
		tags     []string
	}{
		{exe("secretref"), "./cmd/secretref", []string{"-tags", "acceptance"}},
		{exe("printenv-helper"), "./acceptance/helper/printenv-helper", nil},
		{exe("docker"), "./acceptance/helper/printenv-helper", nil},
	}
	for _, b := range builds {
		args := append([]string{"build"}, b.tags...)
		args = append(args, "-o", filepath.Join(dir, b.out), b.pkg)
		cmd := exec.Command("go", args...)
		cmd.Dir = ".."
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return nil
}

func writeFakeSecrets(path string) error {
	field, err := json.Marshal(map[string]string{"password": canary})
	if err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{
		"keyring://app/db_password":           canary,
		"gcp-sm://my-project/api-key":         canary,
		"aws-sm://eu-west-1/prod/db#password": string(field),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func TestScripts(t *testing.T) {
	bin := os.Getenv(binEnv)
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Setup: func(env *testscript.Env) error {
			env.Setenv("PATH", bin+string(os.PathListSeparator)+env.Getenv("PATH"))
			env.Setenv("SECRETREF_TEST_FAKE_FILE", filepath.Join(bin, "fake.json"))
			return nil
		},
		Condition: func(cond string) (bool, error) {
			if cond != "controlling-tty" {
				return false, fmt.Errorf("unknown condition %q", cond)
			}
			f, err := os.Open("/dev/tty")
			if err != nil {
				return false, nil
			}
			return true, f.Close()
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"term-bg":     cmdTermBackground,
			"no-canary":   cmdNoCanary,
			"exit-code":   cmdExitCode,
			"waitfile":    cmdWaitFile,
			"no-override": cmdNoOverride,
			"crlf":        cmdCRLF,
		},
	})
}

// cmdNoCanary fails if the canary appears in the last command's stdout or
// stderr, or in any file under $WORK other than the helper's child-env.txt.
func cmdNoCanary(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 0 {
		ts.Fatalf("usage: no-canary")
	}
	for _, name := range []string{"stdout", "stderr"} {
		if strings.Contains(ts.ReadFile(name), canary) {
			ts.Fatalf("canary found in %s", name)
		}
	}
	work := ts.Getenv("WORK")
	err := filepath.WalkDir(work, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "child-env.txt" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), canary) {
			return fmt.Errorf("canary found in %s", path)
		}
		return nil
	})
	if err != nil {
		ts.Fatalf("%v", err)
	}
}

// cmdExitCode runs a program like exec and checks its exit code.
func cmdExitCode(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 2 {
		ts.Fatalf("usage: exit-code <code> <program> [args...]")
	}
	want, err := strconv.Atoi(args[0])
	if err != nil {
		ts.Fatalf("exit-code: %v", err)
	}
	got := 0
	if err := ts.Exec(args[1], args[2:]...); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			ts.Fatalf("exit-code: %v", err)
		}
		got = ee.ExitCode()
	}
	if got != want {
		ts.Fatalf("exit code %d, want %d", got, want)
	}
}

// cmdWaitFile waits up to 10 seconds for a file to exist.
func cmdWaitFile(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: waitfile <path>")
	}
	path := ts.MkAbs(args[0])
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}
	ts.Fatalf("%s did not appear", args[0])
}

// cmdNoOverride fails if a compose override file is left in the temporary
// directory.
func cmdNoOverride(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 0 {
		ts.Fatalf("usage: no-override")
	}
	matches, err := filepath.Glob(filepath.Join(ts.Getenv("WORK"), ".tmp", "secretref-compose-override-*"))
	if err != nil {
		ts.Fatalf("%v", err)
	}
	if len(matches) > 0 {
		ts.Fatalf("override left behind: %v", matches)
	}
}

// cmdTermBackground sends SIGTERM to every background command; testscript's
// kill only knows INT and KILL.
func cmdTermBackground(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 0 {
		ts.Fatalf("usage: term-bg")
	}
	for _, c := range ts.BackgroundCmds() {
		if err := c.Process.Signal(syscall.SIGTERM); err != nil {
			ts.Fatalf("%v", err)
		}
	}
}

// cmdCRLF copies src to dst with every LF turned into CRLF.
func cmdCRLF(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: crlf <src> <dst>")
	}
	data := strings.ReplaceAll(ts.ReadFile(args[0]), "\n", "\r\n")
	if err := os.WriteFile(ts.MkAbs(args[1]), []byte(data), 0o600); err != nil {
		ts.Fatalf("%v", err)
	}
}
