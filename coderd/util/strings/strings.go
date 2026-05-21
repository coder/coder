package strings

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/acarl005/stripansi"
	"github.com/microcosm-cc/bluemonday"
)

// EmptyToNil returns a `nil` for an empty string, or a pointer to the string
// otherwise. Useful when needing to treat zero values as nil in APIs.
func EmptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// JoinWithConjunction joins a slice of strings with commas except for the last
// two which are joined with "and".
func JoinWithConjunction(s []string) string {
	last := len(s) - 1
	if last == 0 {
		return s[last]
	}
	return fmt.Sprintf("%s and %s",
		strings.Join(s[:last], ", "),
		s[last],
	)
}

type TruncateOption int

func (o TruncateOption) String() string {
	switch o {
	case TruncateWithEllipsis:
		return "TruncateWithEllipsis"
	case TruncateWithFullWords:
		return "TruncateWithFullWords"
	default:
		return fmt.Sprintf("TruncateOption(%d)", o)
	}
}

const (
	// TruncateWithEllipsis adds a Unicode ellipsis character to the end of the string.
	TruncateWithEllipsis TruncateOption = 1 << 0
	// TruncateWithFullWords ensures that words are not split in the middle.
	// As a special case, if there is no word boundary, the string is truncated.
	TruncateWithFullWords TruncateOption = 1 << 1
)

// Truncate truncates s to n runes.
// Additional behaviors can be specified using TruncateOptions.
func Truncate(s string, n int, opts ...TruncateOption) string {
	var options TruncateOption
	for _, opt := range opts {
		options |= opt
	}
	if n < 1 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}

	maxLen := n
	if options&TruncateWithEllipsis != 0 {
		maxLen--
	}
	var sb strings.Builder
	if options&TruncateWithFullWords != 0 {
		// Convert the rune-safe prefix to a string, then find
		// the last word boundary (byte offset within that prefix).
		truncated := string(runes[:maxLen])
		lastWordBoundary := strings.LastIndexFunc(truncated, unicode.IsSpace)
		if lastWordBoundary < 0 {
			_, _ = sb.WriteString(truncated)
		} else {
			_, _ = sb.WriteString(truncated[:lastWordBoundary])
		}
	} else {
		_, _ = sb.WriteString(string(runes[:maxLen]))
	}

	if options&TruncateWithEllipsis != 0 {
		_, _ = sb.WriteString("…")
	}
	return sb.String()
}

var bmPolicy = bluemonday.StrictPolicy()

// UISanitize sanitizes a string for display in the UI.
// The following transformations are applied, in order:
// - HTML tags are removed using bluemonday's strict policy.
// - ANSI escape codes are stripped using stripansi.
// - Consecutive backslashes are replaced with a single backslash.
// - Non-printable characters are removed.
// - Whitespace characters are replaced with spaces.
// - Multiple spaces are collapsed into a single space.
// - Leading and trailing whitespace is trimmed.
func UISanitize(in string) string {
	if unq, err := strconv.Unquote(`"` + in + `"`); err == nil {
		in = unq
	}
	in = bmPolicy.Sanitize(in)
	in = stripansi.Strip(in)
	var b strings.Builder
	var spaceSeen bool
	for _, r := range in {
		if unicode.IsSpace(r) {
			if !spaceSeen {
				_, _ = b.WriteRune(' ')
				spaceSeen = true
			}
			continue
		}
		spaceSeen = false
		if unicode.IsPrint(r) {
			_, _ = b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// Capitalize returns s with its first rune upper-cased. It is safe for
// multi-byte UTF-8 characters, unlike naive byte-slicing approaches.
func Capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}
