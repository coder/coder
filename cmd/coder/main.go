package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"time"
	_ "time/tzdata"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

func main() {
	rand.Seed(time.Now().UnixMicro())

	var cmd cli.RootCmd
	i := clibase.Invokation{
		Args:    os.Args[1:],
		Command: cmd.Command(cli.AGPL()),
		Environ: clibase.ParseEnviron(os.Environ(), ""),
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Stdin:   os.Stdin,
	}

	err := i.Run()
	if err != nil {
		if errors.Is(err, cliui.Canceled) {
			os.Exit(1)
		}
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
