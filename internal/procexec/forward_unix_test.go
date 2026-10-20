//go:build !windows

package procexec

import (
	"os"
	"syscall"
	"testing"
)

func TestShouldForward(t *testing.T) {
	tests := []struct {
		name            string
		sig             os.Signal
		isTTY           bool
		foreground, own int
		want            bool
	}{
		{"no tty: interrupt", syscall.SIGINT, false, 0, 7, true},
		{"no tty: hangup", syscall.SIGHUP, false, 0, 7, true},
		{"no tty: quit", syscall.SIGQUIT, false, 0, 7, true},
		{"no tty: terminate", syscall.SIGTERM, false, 0, 7, true},
		{"foreground tty: interrupt", syscall.SIGINT, true, 7, 7, false},
		{"foreground tty: hangup", syscall.SIGHUP, true, 7, 7, false},
		{"foreground tty: quit", syscall.SIGQUIT, true, 7, 7, false},
		{"foreground tty: terminate", syscall.SIGTERM, true, 7, 7, true},
		{"background tty: interrupt", syscall.SIGINT, true, 9, 7, true},
		{"background tty: hangup", syscall.SIGHUP, true, 9, 7, true},
	}
	for _, tt := range tests {
		if got := shouldForward(tt.sig, tt.isTTY, tt.foreground, tt.own); got != tt.want {
			t.Errorf("%s: shouldForward = %v, want %v", tt.name, got, tt.want)
		}
	}
}
