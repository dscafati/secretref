package resolve

import "strings"

// expand replaces ${NAME}, ${NAME:-default} and $NAME in s using lookup, and
// turns \$ into a literal $. Undefined names expand to the empty string. The
// default in ${NAME:-default} is literal text up to the first "}": it is not
// expanded, so ${A:-$B} yields the characters "$B" when A is unset.
func expand(s string, lookup func(string) (string, bool)) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && s[i+1] == '$':
			b.WriteByte('$')
			i++
		case c == '$' && i+1 < len(s) && s[i+1] == '{':
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				b.WriteString(s[i:])
				return b.String()
			}
			body := s[i+2 : i+2+end]
			name, def, hasDef := strings.Cut(body, ":-")
			if !validName(name) {
				b.WriteString(s[i : i+3+end])
			} else {
				v, ok := lookup(name)
				if (!ok || v == "") && hasDef {
					v = def
				}
				b.WriteString(v)
			}
			i += 2 + end
		case c == '$' && i+1 < len(s) && isNameStart(s[i+1]):
			j := i + 2
			for j < len(s) && isNameChar(s[j]) {
				j++
			}
			v, _ := lookup(s[i+1 : j])
			b.WriteString(v)
			i = j - 1
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isNameChar(c byte) bool { return isNameStart(c) || (c >= '0' && c <= '9') }

func validName(n string) bool {
	if n == "" || !isNameStart(n[0]) {
		return false
	}
	for i := 1; i < len(n); i++ {
		if !isNameChar(n[i]) {
			return false
		}
	}
	return true
}
