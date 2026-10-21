package cli_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dscafati/secretref/internal/cli"
	"github.com/dscafati/secretref/internal/provider"
	"github.com/dscafati/secretref/internal/ref"
	"github.com/dscafati/secretref/internal/secret"
)

// expiringWriter stands in for a keyring that never answers: it records the
// deadline it was given and fails the way a provider does once that deadline
// passes, without making the test wait for it.
type expiringWriter struct {
	deadline time.Time
	ok       bool
}

func (w *expiringWriter) expire(ctx context.Context) error {
	w.deadline, w.ok = ctx.Deadline()
	return provider.Interrupted(expiredCtx(ctx))
}

func (w *expiringWriter) Set(ctx context.Context, _ ref.Ref, _ secret.Secret) error {
	return w.expire(ctx)
}

func (w *expiringWriter) Delete(ctx context.Context, _ ref.Ref) error {
	return w.expire(ctx)
}

// expiredCtx returns a context derived from ctx whose deadline has passed.
func expiredCtx(ctx context.Context) context.Context {
	expired, cancel := context.WithDeadline(ctx, time.Unix(0, 0))
	cancel()
	return expired
}

func TestWriteCommandsTimeOut(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"set", "keyring://app/new"}, "secretref: store keyring://app/new: timed out after 30s\n"},
		{[]string{"delete", refB}, "secretref: delete " + refB + ": timed out after 30s\n"},
	}
	for _, tt := range tests {
		t.Run(tt.args[0], func(t *testing.T) {
			h := newHarness(t)
			h.stdin = canary + "\n"
			w := &expiringWriter{}
			d := h.deps()
			d.Writers = map[string]provider.Writer{"keyring": w}

			start := time.Now()
			code := cli.Execute(context.Background(), tt.args, d)
			end := time.Now()
			if code != 1 {
				t.Fatalf("exit code = %d, want 1\nstderr:\n%s", code, h.stderr.String())
			}
			if !w.ok {
				t.Fatal("writer got a context without a deadline")
			}
			if w.deadline.Before(start.Add(30*time.Second)) || w.deadline.After(end.Add(30*time.Second)) {
				t.Errorf("deadline is %v after start, want 30s", w.deadline.Sub(start))
			}
			if diff := cmp.Diff(tt.want, h.stderr.String()); diff != "" {
				t.Errorf("stderr (-want +got):\n%s", diff)
			}
			if strings.Contains(h.stderr.String()+h.stdout.String(), canary) {
				t.Error("output leaked the canary")
			}
		})
	}
}
