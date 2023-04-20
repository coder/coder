package testutil

import (
	"flag"
	"testing"
)

// We can't run timing-sensitive tests in CI because of the
// great variance in runner performance. Instead of not testing timing at all,
// we relegate it to humans manually running certain tests with the "-timing"
// flag from time to time.
//
// Eventually, we should run all timing tests in a self-hosted runner.

var timingFlag = flag.Bool("timing", false, "run timing-sensitive tests")

func SkipIfNotTiming(t *testing.T) {
	if !*timingFlag {
		t.Skip("skipping timing-sensitive test")
	}
}
