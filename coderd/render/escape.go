package render

import "strings"

// Character classes for EscapeMarkdown. They are split by where a character
// carries structural meaning, which also keeps escaping away from the enum-like
// label values that notification body templates compare with `eq`.
const (
	// inlineCritical characters can produce a link, an image, an angle autolink,
	// or forge an escape from anywhere in a value, so they are always escaped.
	// None of them appears in a control label value such as "user_override".
	inlineCritical = `\[]()!<`

	// blockStart characters carry structural meaning only as the first
	// non-space character of a line, so they are escaped only there. Escaping
	// them everywhere would corrupt values such as "bobby-workspace" and "1.5".
	blockStart = `#-+.>|`

	// foldStart characters also carry meaning only at the start of a line, but
	// neither renderer honors a backslash before them, so escaping would leave
	// a literal backslash in the output. The preceding line break is replaced
	// with a space instead, which denies them the line-start position.
	foldStart = `=~`
)

// EscapeMarkdown neutralizes Markdown structure in an untrusted value so that it
// renders as literal text through both HTMLFromNotificationMarkdown and
// PlaintextFromMarkdown.
//
// Emphasis characters ("*", "_" and backtick) are deliberately left alone. They
// can only produce <em>, <strong>, <del> or <code>, never a link or a heading,
// and escaping "_" would corrupt label values such as "user_override" that body
// templates compare with `eq`.
//
// Line breaks are preserved so multi-line values keep their shape. Other control
// characters are dropped: they have no display value and are the carrier for
// SMTP header injection.
func EscapeMarkdown(s string) string {
	if s == "" {
		return s
	}

	lines := strings.Split(stripControl(s), "\n")
	var b strings.Builder
	b.Grow(len(s) + len(s)/8)

	for i, line := range lines {
		if i > 0 {
			// A fold-start line is joined to the previous one so it is no longer
			// in leading position.
			if opensFoldConstruct(line) {
				_ = b.WriteByte(' ')
			} else {
				_ = b.WriteByte('\n')
			}
		}
		_, _ = b.WriteString(escapeLine(line))
	}
	return b.String()
}

// stripControl replaces horizontal whitespace controls with spaces, drops the
// remaining control characters, and keeps line breaks. Carriage returns are
// folded rather than kept so a value cannot terminate an SMTP header.
func stripControl(s string) string {
	if strings.IndexFunc(s, isStrippable) < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			_, _ = b.WriteRune(r)
		case r == '\r' || r == '\t' || r == '\v' || r == '\f':
			_, _ = b.WriteRune(' ')
		case r < 0x20 || r == 0x7f:
			// Dropped.
		default:
			_, _ = b.WriteRune(r)
		}
	}
	return b.String()
}

func isStrippable(r rune) bool {
	return r != '\n' && (r < 0x20 || r == 0x7f)
}

// escapeLine escapes every inlineCritical character in the line, plus a single
// blockStart character in leading position.
func escapeLine(line string) string {
	var b strings.Builder
	b.Grow(len(line))

	leading := true
	for _, r := range line {
		switch {
		case r < 0x80 && strings.ContainsRune(inlineCritical, r):
			_ = b.WriteByte('\\')
			_, _ = b.WriteRune(r)
		case leading && r == ' ':
			// Indentation keeps the next character in leading position.
			_, _ = b.WriteRune(r)
			continue
		case leading && r < 0x80 && strings.ContainsRune(blockStart, r):
			_ = b.WriteByte('\\')
			_, _ = b.WriteRune(r)
		default:
			_, _ = b.WriteRune(r)
		}
		leading = false
	}
	return b.String()
}

// opensFoldConstruct reports whether a line's first non-space character is a
// foldStart character, meaning the line could act as a Setext underline or open
// a tilde-fenced code block.
func opensFoldConstruct(line string) bool {
	t := strings.TrimLeft(line, " ")
	if t == "" {
		return false
	}
	return strings.ContainsRune(foldStart, rune(t[0]))
}
