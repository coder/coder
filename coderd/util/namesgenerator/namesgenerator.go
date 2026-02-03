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
	"math/rand/v2"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/brianvoe/gofakeit/v7"
)

// maxNameLen is the maximum length for names. Many places in Coder have a 32
// character limit for names (e.g. usernames, workspace names).
const maxNameLen = 32

// counter provides unique suffixes for UniqueName functions.
var counter atomic.Int64

// NameWith returns a random name with a custom delimiter.
// Names are not guaranteed to be unique.
func NameWith(delim string) string {
	const seed = 0 // gofakeit will use a random crypto seed.
	faker := gofakeit.New(seed)
	adjective := strings.ToLower(faker.AdjectiveDescriptive())
	last := strings.ToLower(faker.LastName())
	return adjective + delim + last
}

// NameDigitWith returns a random name with a single random digit suffix (1-9),
// in the format "[adjective][delim][surname][digit]" e.g. "happy_smith9".
// Provides some collision resistance while keeping names short and clean.
// Not guaranteed to be unique.
func NameDigitWith(delim string) string {
	const (
		minDigit = 1
		maxDigit = 9
	)
	//nolint:gosec // The random digit doesn't need to be cryptographically secure.
	return NameWith(delim) + strconv.Itoa(rand.IntN(maxDigit-minDigit+1))
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
