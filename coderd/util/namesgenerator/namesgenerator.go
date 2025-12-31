// Package namesgenerator generates random names.
//
// This is a fork of github.com/moby/moby/pkg/namesgenerator that uses a
// monotonically increasing counter instead of a random suffix to guarantee
// uniqueness within a process for some functions. This alleviates name
// collisions in parallel tests. It also provides length-limited names
// for limitations commonly found in Coder (e.g. usernames, workspace names).
package namesgenerator

import (
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/brianvoe/gofakeit/v7"
)

// maxNameLen is the maximum length for names in tests. Many places in Coder
// have a 32 character limit for names (e.g. usernames, workspace names).
const maxNameLen = 32

// counter is used to generate unique suffixes for names.
var counter atomic.Int64

// GenerateDelimited generates a string of an adjective and last name joined
// by the given delimiter.
//
// WARNING: The names returned are not guaranteed to be unique. Use GetRandomName
// or testutil.GetRandomName if unique names are required.
func GenerateDelimited(delim string) string {
	const seed = 0 // gofakeit will use a random crypto seed.
	return generateDelimited(delim, seed)
}

//nolint:revive // Differs in capitalization so the public API doesn't need to provide a seed.
func generateDelimited(delim string, seed uint64) string {
	faker := gofakeit.New(seed)
	adjective := strings.ToLower(faker.AdjectiveDescriptive())
	last := strings.ToLower(faker.LastName())
	return strings.Join([]string{adjective, last}, delim)
}

func generateDelimitedUnique(delim string, seed uint64) string {
	return generateDelimited(delim, seed) + strconv.FormatInt(counter.Add(1), 10)
}

// GetRandomName generates a random name. The name is formatted as "adjective_surname"
// with a monotonically increasing suffix to guarantee uniqueness within the process.
// It is truncated to maxNameLen characters if necessary.
func GetRandomName() string {
	const seed = 0 // gofakeit will use a random crypto seed.
	return getRandomName("_", seed)
}

//nolint:revive // Differs in capitalization so the public API doesn't need to provide a seed.
func getRandomName(delim string, seed uint64) string {
	generatedWithSuffix := generateDelimitedUnique(delim, seed)
	return truncate(generatedWithSuffix, maxNameLen)
}

// GetRandomNameHyphenated is like GetRandomName but uses hyphens instead of
// underscores.
func GetRandomNameHyphenated() string {
	const seed = 0 // gofakeit will use a random crypto seed.
	return getRandomName("-", seed)
}

// truncate truncates a name to maxLen characters. It assumes the name ends with
// a numeric suffix and preserves it, truncating the base name portion instead.
// If there are enough names generated, this would truncate the entire name, but
// that's unlikely to happen in practice.
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
