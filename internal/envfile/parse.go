package envfile

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/dscafati/secretref/internal/ref"
)

// annotationPrefix detects a comment that is meant as an annotation, so that a
// malformed one is reported instead of silently ignored.
var annotationPrefix = regexp.MustCompile(`^\s*#\s*secretref:`)

// annotationRE matches a well-formed annotation and captures the reference.
var annotationRE = regexp.MustCompile(`^\s*#\s*secretref:\s*(\S+)\s*$`)

const (
	noVariableBelow = "annotation has no variable directly below it"
	skippedLine     = "skipped unparseable line"
)

// Parse reads the env file name from r. name is used in errors, in Entry.File,
// and in Warning.File.
//
// A line that is not a valid KEY=VALUE line is skipped with a Warning, as
// other dotenv loaders skip it, unless it is directly below an annotation:
// then it is a *ParseError. Malformed annotations are always errors.
//
// For QuoteDouble values, escapes \n \r \t \" and \\ are processed. The \$
// escape is NOT processed: Entry.Value keeps the two characters \$ so that the
// expansion step can tell a literal dollar sign from an expandable one, and
// unescapes it itself. Any other backslash sequence is kept as written.
func Parse(name string, r io.Reader) ([]Entry, []Warning, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", name, err)
	}
	p := &parser{name: name, lines: splitLines(string(data))}
	return p.run()
}

// splitLines splits on \n and strips one trailing \r from each line.
func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	// A final newline yields an empty last element that is not a line.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

type parser struct {
	name  string
	lines []string
	pos   int // index of the next line to read

	pending     *ref.Ref // annotation waiting for its key
	pendingLine int
}

func (p *parser) errAt(line int, format string, args ...any) error {
	return &ParseError{File: p.name, Line: line, Msg: fmt.Sprintf(format, args...)}
}

func (p *parser) run() ([]Entry, []Warning, error) {
	var entries []Entry
	var warnings []Warning
	for p.pos < len(p.lines) {
		lineNo := p.pos + 1
		text := p.lines[p.pos]
		p.pos++

		trimmed := strings.TrimSpace(text)
		switch {
		case trimmed == "":
			if err := p.requireNoPending(); err != nil {
				return nil, nil, err
			}
		case strings.HasPrefix(trimmed, "#"):
			if err := p.comment(text, lineNo); err != nil {
				return nil, nil, err
			}
		default:
			next := p.pos
			e, err := p.assignment(text, lineNo)
			if err != nil {
				if p.pending != nil {
					return nil, nil, err
				}
				// Skip only this line, even if an unterminated quote read
				// further; the following lines parse on their own.
				p.pos = next
				warnings = append(warnings, Warning{File: p.name, Line: lineNo, Msg: skippedLine})
				continue
			}
			e.Ref, p.pending = p.pending, nil
			entries = append(entries, e)
		}
	}
	if err := p.requireNoPending(); err != nil {
		return nil, nil, err
	}
	return entries, warnings, nil
}

func (p *parser) requireNoPending() error {
	if p.pending != nil {
		return p.errAt(p.pendingLine, noVariableBelow)
	}
	return nil
}

// comment handles a comment line: an annotation or an ordinary comment.
func (p *parser) comment(text string, lineNo int) error {
	if err := p.requireNoPending(); err != nil {
		return err
	}
	if !annotationPrefix.MatchString(text) {
		return nil
	}
	m := annotationRE.FindStringSubmatch(text)
	if m == nil {
		return p.errAt(lineNo, "annotation needs exactly one reference")
	}
	r, err := ref.Parse(m[1])
	if err != nil {
		// SECURITY: never echo the annotation text; a secret may have been
		// pasted there. Only a recognized scheme name is safe to show.
		return p.errAt(lineNo, "invalid reference%s", schemeHint(m[1]))
	}
	p.pending, p.pendingLine = &r, lineNo
	return nil
}

func isKeyStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isKeyChar(c byte) bool {
	return isKeyStart(c) || (c >= '0' && c <= '9') || c == '.' || c == '-'
}

func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// assignment parses KEY=VALUE starting on lineNo. Multi-line double-quoted
// values consume further lines.
func (p *parser) assignment(text string, lineNo int) (Entry, error) {
	s := strings.TrimLeft(text, " \t")
	if rest, ok := strings.CutPrefix(s, "export"); ok && rest != "" && isBlank(rest[0]) {
		s = strings.TrimLeft(rest, " \t")
	}

	n := 0
	for n < len(s) && (n == 0 && isKeyStart(s[n]) || n > 0 && isKeyChar(s[n])) {
		n++
	}
	if n == 0 {
		return Entry{}, p.errAt(lineNo, "invalid variable name")
	}
	if n < len(s) && !isBlank(s[n]) && s[n] != '=' {
		return Entry{}, p.errAt(lineNo, "invalid variable name")
	}
	key, s := s[:n], strings.TrimLeft(s[n:], " \t")
	if s == "" || s[0] != '=' {
		return Entry{}, p.errAt(lineNo, "expected '=' after variable name")
	}
	s = s[1:]
	if t := strings.TrimLeft(s, " \t"); t != "" && (t[0] == '\'' || t[0] == '"') {
		s = t
	}

	e := Entry{File: p.name, Line: lineNo, Key: key}
	var err error
	switch {
	case strings.HasPrefix(s, "'"):
		e.Quote = QuoteSingle
		e.Value, err = p.singleQuoted(s[1:], lineNo)
	case strings.HasPrefix(s, `"`):
		e.Quote = QuoteDouble
		e.Value, err = p.doubleQuoted(s[1:], lineNo)
	default:
		e.Value = unquoted(s)
	}
	if err != nil {
		return Entry{}, err
	}
	return e, nil
}

// unquoted trims s and drops an inline comment: a # preceded by a space or tab.
func unquoted(s string) string {
	for i := range len(s) {
		if s[i] == '#' && i > 0 && isBlank(s[i-1]) {
			s = s[:i]
			break
		}
	}
	return strings.TrimSpace(s)
}

// singleQuoted reads a literal single-quoted value that starts in s (the text
// after the opening quote). It may continue on following lines; no escapes are
// processed.
func (p *parser) singleQuoted(s string, lineNo int) (string, error) {
	var b strings.Builder
	for {
		if end := strings.IndexByte(s, '\''); end >= 0 {
			b.WriteString(s[:end])
			if err := p.afterQuote(s[end+1:], p.pos); err != nil {
				return "", err
			}
			return b.String(), nil
		}
		b.WriteString(s)
		if p.pos >= len(p.lines) {
			return "", p.errAt(lineNo, "unterminated single-quoted value")
		}
		b.WriteByte('\n')
		s = p.lines[p.pos]
		p.pos++
	}
}

// doubleQuoted reads a double-quoted value that starts in s (the text after the
// opening quote) and may continue on following lines.
func (p *parser) doubleQuoted(s string, lineNo int) (string, error) {
	var b strings.Builder
	for {
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch {
			case c == '"':
				if err := p.afterQuote(s[i+1:], p.pos); err != nil {
					return "", err
				}
				return b.String(), nil
			case c == '\\' && i+1 < len(s):
				i++
				b.WriteString(escape(s[i]))
			default:
				b.WriteByte(c)
			}
		}
		if p.pos >= len(p.lines) {
			return "", p.errAt(lineNo, "unterminated double-quoted value")
		}
		b.WriteByte('\n')
		s = p.lines[p.pos]
		p.pos++
	}
}

// escape returns the text for the backslash escape \c inside double quotes.
func escape(c byte) string {
	switch c {
	case 'n':
		return "\n"
	case 'r':
		return "\r"
	case 't':
		return "\t"
	case '"':
		return `"`
	case '\\':
		return `\`
	case '$':
		// Kept escaped so expansion can tell it from an expandable $.
		return `\$`
	}
	return `\` + string(c)
}

// afterQuote checks that only whitespace or a comment follows a closing quote.
func (p *parser) afterQuote(rest string, lineNo int) error {
	t := strings.TrimLeft(rest, " \t")
	if t == "" || (t[0] == '#' && len(t) < len(rest)) {
		return nil
	}
	return p.errAt(lineNo, "unexpected text after quoted value")
}

// schemeHint returns " (scheme X)" when raw starts with a supported scheme X,
// a list of the supported schemes when raw starts with another "<scheme>://",
// and "" otherwise.
//
// SECURITY: an unknown scheme is never returned; it may be pasted secret text.
func schemeHint(raw string) string {
	scheme, _, ok := strings.Cut(raw, "://")
	if !ok {
		return ""
	}
	supported := ref.Schemes()
	if slices.Contains(supported, scheme) {
		return " (scheme " + scheme + ")"
	}
	return ": unknown scheme; supported: " + strings.Join(supported, ", ")
}
