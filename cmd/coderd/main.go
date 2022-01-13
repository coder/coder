package main

import (
	"fmt"
	"os"

	"github.com/coder/coder/coderd/cmd"
)

func main() {
	err := cmd.Root().Execute()
	if err != nil {
		_, _ = fmt.Println(err.Error())
		os.Exit(1)
	}
}
