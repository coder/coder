// Package namesgenerator generates random names.
//
// This package provides functions for generating random names in the format
// "adjective_surname" with various options for delimiters and uniqueness.
//
// For identifiers that must be unique within a process, use UniqueName or
// UniqueNameWith. For display purposes where uniqueness is not required,
// use NameWith.
package namesgenerator

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/brianvoe/gofakeit/v7/data"
)

// maxNameLen is the maximum length for names. Many places in Coder have a 32
// character limit for names (e.g. usernames, workspace names).
const maxNameLen = 32

// counter provides unique suffixes for UniqueName functions.
var counter atomic.Int64

// NameWith returns a random name with a custom delimiter.
// Names are not guaranteed to be unique.
//
// The word lists come from gofakeit's data package. Importing the gofakeit
// package itself would link every faker into the binary.
func NameWith(delim string) string {
	adjectives := data.Word["adjective_descriptive"]
	lastNames := data.Person["last"]
	//nolint:gosec // The names don't need to be cryptographically secure.
	adjective := strings.ToLower(adjectives[rand.IntN(len(adjectives))])
	//nolint:gosec // The names don't need to be cryptographically secure.
	last := strings.ToLower(lastNames[rand.IntN(len(lastNames))])
	return adjective + delim + last
}

// NameDigitWith returns a random name with a two-digit suffix (00-99),
// in the format "[adjective][delim][surname][digit]" e.g. "happy_smith42".
// Provides some collision resistance while keeping names short and clean.
// Not guaranteed to be unique.
func NameDigitWith(delim string) string {
	//nolint:gosec // The random digit doesn't need to be cryptographically secure.
	name := NameWith(delim) + fmt.Sprintf("%02d", rand.IntN(100))
	return truncate(name, maxNameLen)
}

// UniqueName returns a random name with a monotonically increasing suffix,
// guaranteeing uniqueness within the process. The name is truncated to 32
// characters if necessary, preserving the numeric suffix.
func UniqueName() string {
	return UniqueNameWith("_")
}

// UniqueNameWith returns a unique name with a custom delimiter.
// See UniqueName for details on uniqueness guarantees.
func UniqueNameWith(delim string) string {
	name := NameWith(delim) + strconv.FormatInt(counter.Add(1), 10)
	return truncate(name, maxNameLen)
}

// truncate truncates a name to maxLen characters. It assumes the name ends with
// a numeric suffix and preserves it, truncating the base name portion instead.
func truncate(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	// Find where the numeric suffix starts.
	suffixStart := len(name)
	for suffixStart > 0 && name[suffixStart-1] >= '0' && name[suffixStart-1] <= '9' {
		suffixStart--
	}
	base := name[:suffixStart]
	suffix := name[suffixStart:]
	truncateAt := maxLen - len(suffix)
	if truncateAt <= 0 {
		return strconv.Itoa(maxLen) // Fallback, shouldn't happen in practice.
	}
	return base[:truncateAt] + suffix
}
