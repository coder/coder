package testutil

import "testing"

// IsParallel determines whether the current test is running with parallel set.
// The Go standard library does not currently expose this. However, we can easily
// determine this using the fact that a call to t.Setenv() after t.Parallel() will panic.
func IsParallel(t testing.TB) (parallel bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			parallel = true
		}
	}()
	t.Setenv(t.Name()+"_PARALLEL", "")
	return parallel
}
