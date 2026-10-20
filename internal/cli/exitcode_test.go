package cli

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestExitCodeMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, exitOK},
		{"untagged is internal", errors.New("boom"), exitInternal},
		{"usage", usageError("bad"), exitUsage},
		{"tagged cobra error", asUsage(errors.New("accepts 1 arg(s), received 2")), exitUsage},
		{"wrapped exit error", fmt.Errorf("ctx: %w", exitWith(exitResolve)), exitResolve},
		{"asUsage keeps a tagged code", asUsage(exitWith(exitResolve)), exitResolve},
	}
	for _, tt := range tests {
		var stderr bytes.Buffer
		if got := exitCode(tt.err, &stderr); got != tt.want {
			t.Errorf("%s: exitCode = %d, want %d", tt.name, got, tt.want)
		}
	}
}
