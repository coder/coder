package main

import (
	"errors"
	"fmt"
	"os"
	_ "time/tzdata"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliui"
)

func main() {
	cmd, err := cli.Root().ExecuteC()
	if err != nil {
		if errors.Is(err, cliui.Canceled) {
			os.Exit(1)
		}
		cobraErr := cli.FormatCobraError(err, cmd)
		_, _ = fmt.Fprintln(os.Stderr, cobraErr)
		os.Exit(1)
	}
}
