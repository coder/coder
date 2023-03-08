package main

import (
	"os"

	"github.com/coder/coder/cli/cliui"
)

// This script demonstrates cliui's theming.
func main() {
	cliui.Info(os.Stdout, "this is an informational message",
		"cats are related to tigers",
		"tigers like to eat meat",
	)

	cliui.Warn(os.Stdout, "this is a warning",
		"details about the warning",
		"and more details",
	)

	cliui.Error(os.Stdout, "this is an error",
		"details about the error",
		"wow this is a long error message",
	)
}
