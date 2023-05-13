package testutil

import (
	"testing"
)

// We can't run timing-sensitive tests in CI because of the
// great variance in runner performance. Instead of not testing timing at all,
// we relegate it to humans manually running certain tests with the "-timing"
// flag from time to time.
//
// Eventually, we should run all timing tests in a self-hosted runner.

var timing bool

func SkipIfNotTiming(t *testing.T) {
	if !timing {
		t.Skip("skipping timing-sensitive test")
	}
}
