package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"time"
	_ "time/tzdata"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliui"
	entcli "github.com/coder/coder/enterprise/cli"
)

func main() {
	rand.Seed(time.Now().UnixMicro())

	cmd, err := cli.Root(entcli.EnterpriseSubcommands()).ExecuteC()
	if err != nil {
		if errors.Is(err, cliui.Canceled) {
			os.Exit(1)
		}
		cobraErr := cli.FormatCobraError(err, cmd)
		_, _ = fmt.Fprintln(os.Stderr, cobraErr)
		os.Exit(1)
	}
}
