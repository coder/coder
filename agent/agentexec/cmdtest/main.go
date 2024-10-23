package main

import (
	"context"
	"fmt"
	"os"

	"github.com/coder/coder/v2/agent/agentexec"
)

func main() {
	err := agentexec.CLI(context.Background(), os.Args, os.Environ())
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
