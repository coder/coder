package main

import (
	"os"

	"github.com/coder/coder/cli"
)

func main() {
	err := cli.Root().Execute()
	if err != nil {
		os.Exit(1)
	}
}
