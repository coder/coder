package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliui"
)

func main() {
	err := cli.Root().Execute()
	if err != nil {
		if errors.Is(err, cliui.Canceled) {
			os.Exit(1)
		}
		_, _ = fmt.Fprintln(os.Stderr, cliui.Styles.Error.Render(err.Error()))
		os.Exit(1)
	}
}
