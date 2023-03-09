package main

import (
	_ "time/tzdata"

	"github.com/coder/coder/cli"
)

func main() {
	cli.Main(cli.AGPL())
}
