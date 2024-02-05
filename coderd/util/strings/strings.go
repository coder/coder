package strings

import (
	"fmt"
	"strings"
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
