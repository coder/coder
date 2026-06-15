// Command natsbench is a thin entrypoint for the natsbench package's
// Main; all behavior and flags live in that package.
package main

import (
	"fmt"
	"os"

	"github.com/coder/coder/v2/coderd/x/nats/natsbench"
)

func main() {
	if err := natsbench.Main(os.Args, os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "natsbench: %v\n", err)
		os.Exit(1)
	}
}
