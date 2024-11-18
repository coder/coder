package main

import (
	"fmt"
	"os"

	"github.com/coder/coder/v2/agent/agentexec"
)

func main() {
	err := agentexec.CLI(os.Args, os.Environ())
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
