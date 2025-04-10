package testutil

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/pkg/namesgenerator"
)

var n atomic.Int64

const maxNameLen = 32

// GetRandomName returns a random name using moby/pkg/namesgenerator.
// namesgenerator.GetRandomName exposes a retry parameter that appends
// a pseudo-random number between 1 and 10 to its return value.
// While this reduces the probability of collisions, it does not negate them.
// This function calls namesgenerator.GetRandomName without the retry
// parameter and instead increments a monotonically increasing integer
// to the return value.
func GetRandomName(t testing.TB) string {
	t.Helper()
	name := namesgenerator.GetRandomName(0)
	return incSuffix(name, n.Add(1), maxNameLen)
}

// GetRandomNameHyphenated is as GetRandomName but uses a hyphen "-" instead of
// an underscore.
func GetRandomNameHyphenated(t testing.TB) string {
	t.Helper()
	name := namesgenerator.GetRandomName(0)
	name = strings.ReplaceAll(name, "_", "-")
	return incSuffix(name, n.Add(1), maxNameLen)
}

func incSuffix(s string, num int64, maxLen int) string {
	suffix := strconv.FormatInt(num, 10)
	if len(s)+len(suffix) <= maxLen {
		return s + suffix
	}
	stripLen := (len(s) + len(suffix)) - maxLen
	stripIdx := len(s) - stripLen
	if stripIdx < 0 {
		return ""
	}
	s = s[:stripIdx]
	return s + suffix
}
