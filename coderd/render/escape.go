package render

import "strings"

// Character classes for EscapeMarkdown. They are split by where a character
// carries structural meaning, which also keeps escaping away from the enum-like
// label values that notification body templates compare with `eq`.
const (
	// inlineCritical characters can produce a link, an image, an angle autolink,
	// or forge an escape from anywhere in a value, so they are always escaped.
	// None of them appears in a control label value such as "user_override".
	//
	// Backtick is here rather than in blockStart because a fence's info string
	// is an HTML sink: gomarkdown writes it into class="language-..." without
	// escaping, and html.SkipHTML does not apply because the node is a
	// CodeBlock rather than an HTMLBlock. A value that closes the attribute and
	// the tag injects live markup into the rendered email. Escaping only the
	// leading backtick is not enough either, because the two that remain open
	// an inline code span.
	inlineCritical = "\\[]()!<`"

	// blockStart characters carry structural meaning only as the first
	// non-space character of a line, so they are escaped only there. Escaping
	// them everywhere would corrupt values such as "bobby-workspace" and "1.5".
	blockStart = `#-+.>|`

	// leadingEmphasis characters carry inline meaning anywhere but also open a
	// block construct in leading position: "* " starts a bullet list, and three
	// or more of either character starts a thematic break. They are escaped in
	// leading position for the same reason as blockStart, which costs emphasis
	// that begins on a line boundary and keeps "user_override" intact.
	leadingEmphasis = `*_`

	// foldStart characters also carry meaning only at the start of a line, but
	// glamour does not honor a backslash before them, so escaping would leave a
	// literal backslash in the plaintext part. The preceding line break is
	// replaced with a space instead, which denies them the line-start position.
	//
	// ":" opens a definition list, and it also opens a GFM table delimiter row
	// such as ":-- | --:". Escaping "|" does not close that second case,
	// because a delimiter row's pipes are mid-line and "|" is escaped only in
	// leading position.
	foldStart = `=~:`

	// maxLeadingSpaces is the widest indentation a line may keep. Four spaces
	// open an indented code block, and there is no escape for a space, so the
	// run is truncated instead. Three is the most CommonMark allows before a
	// block marker while still treating it as indentation.
	maxLeadingSpaces = 3
)

// EscapeMarkdown neutralizes Markdown structure in an untrusted value so that it
// renders as literal text through both HTMLFromNotificationMarkdown and
// PlaintextFromMarkdown.
//
// Emphasis characters ("*" and "_") are deliberately left alone away from a
// line's leading position. They can only produce <em>, <strong> or <del> there,
// never a link or a heading, and escaping "_" everywhere would corrupt label
// values such as "user_override" that body templates compare with `eq`. In
// leading position they do open a block construct, so they are escaped, see
// leadingEmphasis.
//
// Line breaks are preserved so multi-line values keep their shape. Other control
// characters are dropped: they have no display value and are the carrier for
// SMTP header injection.
//
// Two residuals this cannot reach, both because they depend on where the value
// lands rather than on what it contains:
//
//   - A template that wraps the value in a code span. CommonMark does not
//     process escapes inside one, so the backslashes emitted here render as
//     literal text.
//   - The value's own first line. Folding a fold-start line onto its
//     predecessor only works for line breaks this function emitted, so a value
//     beginning "===" is still a Setext underline for whatever the template put
//     on the line above.
//
// Both close under placeholder substitution, which resolves after rendering and
// therefore knows the destination context.
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
// blockStart or leadingEmphasis character in leading position, plus the "."
// that closes an ordered-list marker. Leading indentation is truncated to
// maxLeadingSpaces so the line cannot become an indented code block.
func escapeLine(line string) string {
	var b strings.Builder
	b.Grow(len(line))

	leading := true
	// spaces counts the indentation emitted so far, to cap it.
	spaces := 0
	// digitRun reports whether the line so far is nothing but indentation and
	// digits, which is the only position where "." opens an ordered list.
	digitRun := false
	for i, r := range line {
		switch {
		case r < 0x80 && strings.ContainsRune(inlineCritical, r):
			_ = b.WriteByte('\\')
			_, _ = b.WriteRune(r)
		case leading && r == ' ':
			// Indentation keeps the next character in leading position, but
			// only the first maxLeadingSpaces of it are emitted: four spaces
			// open an indented code block and a space cannot be escaped.
			if spaces < maxLeadingSpaces {
				_, _ = b.WriteRune(r)
				spaces++
			}
			continue
		case leading && r < 0x80 && strings.ContainsRune(blockStart+leadingEmphasis, r):
			_ = b.WriteByte('\\')
			_, _ = b.WriteRune(r)
		case digitRun && r == '.' && closesMarker(line, i):
			// The "1." of an ordered list. Its sibling "1)" needs no case
			// because ")" is inlineCritical and is always escaped.
			_ = b.WriteByte('\\')
			_, _ = b.WriteRune(r)
		default:
			_, _ = b.WriteRune(r)
		}
		digitRun = (leading || digitRun) && r >= '0' && r <= '9'
		leading = false
	}
	return b.String()
}

// closesMarker reports whether the single-byte list-marker delimiter at i is
// followed by a space or ends the line. CommonMark requires that of a marker,
// which is what keeps a value such as "1.5" out of the escaped set: templates
// compare numeric label values with `eq`, so escaping one changes control flow.
// Tabs need no handling here because stripControl has already folded them into
// spaces.
func closesMarker(line string, i int) bool {
	return i+1 == len(line) || line[i+1] == ' '
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
