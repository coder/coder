package testutil

import (
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/pkg/namesgenerator"
)

var (
	n atomic.Int64
)

// GetRandomName returns a random name using moby/pkg/namesgenerator.
// namesgenerator.GetRandomName exposes a retry parameter that appends
// a pseudo-random number between 1 and 10 to its return value.
// While this reduces the probability of collisions, it does not negate them.
// This function calls namesgenerator.GetRandomName without the retry
// parameter and instead increments a monotonically increasing integer
// to the return value.
func GetRandomName(t testing.TB) string {
	t.Helper()
	return namesgenerator.GetRandomName(0) + strconv.FormatInt(n.Add(1), 10)
}
