package coderdtest

import (
	"flag"
	"fmt"
	"runtime"
	"strconv"
	"testing"
)

const (
	// MaxTestParallelism is set to match our MakeFile's `make test` target.
	MaxTestParallelism = 8
)

// init defines the default parallelism for tests, capping it to MaxTestParallelism.
// Any user-provided value for -test.parallel will override this.
func init() {
	// Setup the test flags.
	testing.Init()

	// info is used for debugging panics in this init function.
	info := "Resolve the issue in the file initflags.go"
	_, file, line, ok := runtime.Caller(0)
	if ok {
		info = fmt.Sprintf("Resolve the issue in the file %s:%d", file, line)
	}

	// Lookup the test.parallel flag's value, and cap it to MaxTestParallelism. This
	// all happens before `flag.Parse()`, so any user-provided value will overwrite
	// whatever we set here.
	par := flag.CommandLine.Lookup("test.parallel")
	if par == nil {
		// This should never happen. If you are reading this message because of a panic,
		// just comment out the panic and add a `return` statement instead.
		msg := "no 'test.parallel' flag found, unable to set default parallelism"
		panic(msg + "\n" + info)
	}

	parValue, err := strconv.ParseInt(par.Value.String(), 0, 64)
	if err != nil {
		// This should never happen, but if it does, panic with a useful message. If you
		// are reading this message because of a panic, that means the default value for
		// -test.parallel is not an integer. A safe fix is to comment out the panic. This
		// will assume the default value of '0', and replace it with MaxTestParallelism.
		// Which is not ideal, but at least tests will run.
		msg := fmt.Sprintf("failed to parse test.parallel: %v", err)

		panic(msg + "\n" + info)
	}

	if parValue > MaxTestParallelism {
		_ = par.Value.Set(fmt.Sprintf("%d", MaxTestParallelism))
	}
}
