package coderdtest

import (
	"flag"
	"fmt"
	"strconv"
	"testing"
)

const (
	MaxTestParallelism = 1
)

func init() {
	// Setup the test flags.
	testing.Init()

	// Lookup the test.parallel flag's value, and cap it to MaxTestParallelism. This
	// all happens before `flag.Parse()`, so any user-provided value will overwrite
	// whatever we set here.
	par := flag.CommandLine.Lookup("test.parallel")
	parValue, err := strconv.ParseInt(par.Value.String(), 0, 64)
	if err != nil {
		// This should never happen.
		panic(fmt.Sprintf("failed to parse test.parallel: %v", err))
	}

	if parValue > MaxTestParallelism {
		_ = par.Value.Set(fmt.Sprintf("%d", MaxTestParallelism))
	}
}

func init() {
	testing.Init()
	flag.Set("test.parallel", "1") // Disable test parallelism
}
