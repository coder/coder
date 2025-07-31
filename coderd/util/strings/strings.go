package strings

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/acarl005/stripansi"
	"github.com/microcosm-cc/bluemonday"
)

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

// Truncate returns the first n characters of s.
func Truncate(s string, n int) string {
	if n < 1 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
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
