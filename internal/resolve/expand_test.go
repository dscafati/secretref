package resolve

import "testing"

// TestExpandFunc is the reference table for expansion. Only ${NAME} and
// ${NAME:-default} expand, as in Docker Compose; $$ is a literal $.
func TestExpandFunc(t *testing.T) {
	vars := map[string]string{"A": "a", "EMPTY": "", "A_1": "x", "def": "d"}
	lookup := func(k string) (string, bool) { v, ok := vars[k]; return v, ok }
	tests := []struct{ in, want string }{
		// Braced forms expand.
		{"${A}", "a"},
		{"pre-${A}-post", "pre-a-post"},
		{"${A}${A_1}", "ax"},
		{"${NOPE}", ""},
		{"${NOPE:-d}", "d"},
		{"${EMPTY:-d}", "d"},
		{"${A:-d}", "a"},
		{"${NOPE:-}", ""},
		{"${NOPE:-$A}", "$A"},
		{"${NOPE:-a b}", "a b"},
		// Bare $NAME is literal.
		{"$A", "$A"},
		{"$A_1!", "$A_1!"},
		{"abc$def", "abc$def"},
		{"$NOPE", "$NOPE"},
		// $$ is a literal $.
		{"pa$$word", "pa$word"},
		{"$$", "$"},
		{"$$$", "$$"},
		{"$$$$", "$$"},
		{"$${A}", "${A}"},
		{"$$$${A}", "$${A}"},
		{"$$${A}", "$a"},
		// \$ is a literal $.
		{`\$A`, "$A"},
		{`\${A}`, "${A}"},
		{`\$$`, "$$"},
		{`\x`, `\x`},
		// Anything else stays as written.
		{"$", "$"},
		{"a$ b", "a$ b"},
		{"${A", "${A"},
		{"${", "${"},
		{"${}", "${}"},
		{"${1x}", "${1x}"},
		{"${a.b}", "${a.b}"},
		{"${H-I}", "${H-I}"},
		{"$1", "$1"},
		{"$(cmd)", "$(cmd)"},
		{"`cmd`", "`cmd`"},
	}
	for _, tt := range tests {
		if got := expand(tt.in, lookup); got != tt.want {
			t.Errorf("expand(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
