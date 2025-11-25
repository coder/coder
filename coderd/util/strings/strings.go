package strings

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

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

// Truncate truncates s to n characters.
// Additional behaviors can be specified using TruncateOptions.
func Truncate(s string, n int, opts ...TruncateOption) string {
	var options TruncateOption
	for _, opt := range opts {
		options |= opt
	}
	if n < 1 {
		return ""
	}
	if len(s) <= n {
		return s
	}

	maxLen := n
	if options&TruncateWithEllipsis != 0 {
		maxLen--
	}
	var sb strings.Builder
	// If we need to truncate to full words, find the last word boundary before n.
	if options&TruncateWithFullWords != 0 {
		lastWordBoundary := strings.LastIndexFunc(s[:maxLen], unicode.IsSpace)
		if lastWordBoundary < 0 {
			// We cannot find a word boundary. At this point, we'll truncate the string.
			// It's better than nothing.
			_, _ = sb.WriteString(s[:maxLen])
		} else { // lastWordBoundary <= maxLen
			_, _ = sb.WriteString(s[:lastWordBoundary])
		}
	} else {
		_, _ = sb.WriteString(s[:maxLen])
	}

	if options&TruncateWithEllipsis != 0 {
		_, _ = sb.WriteString("…")
	}
	return sb.String()
}

var bmPolicy = bluemonday.StrictPolicy()

// ContainsAny returns true if s contains any of the substrings in the list.
// The search is case-sensitive.
func ContainsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

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
