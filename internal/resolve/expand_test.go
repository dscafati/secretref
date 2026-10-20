package resolve

import "testing"

func TestExpandFunc(t *testing.T) {
	vars := map[string]string{"A": "a", "EMPTY": "", "A_1": "x"}
	lookup := func(k string) (string, bool) { v, ok := vars[k]; return v, ok }
	tests := []struct{ in, want string }{
		{"${A}", "a"},
		{"$A", "a"},
		{"$A_1!", "x!"},
		{"pre-${A}-post", "pre-a-post"},
		{"${NOPE:-d}", "d"},
		{"${EMPTY:-d}", "d"},
		{"${A:-d}", "a"},
		{"${NOPE:-$A}", "$A"},
		{"${NOPE}", ""},
		{"$NOPE", ""},
		{`\$A`, "$A"},
		{`\${A}`, "${A}"},
		{"$", "$"},
		{"a$ b", "a$ b"},
		{"${A", "${A"},
		{"${1x}", "${1x}"},
		{"$1", "$1"},
		{`\x`, `\x`},
	}
	for _, tt := range tests {
		if got := expand(tt.in, lookup); got != tt.want {
			t.Errorf("expand(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
