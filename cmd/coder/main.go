package main

import (
	"fmt"
	"os"
	_ "time/tzdata"

	tea "github.com/sreya/bubbletea"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/cli"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "agent-exec" {
		err := agentexec.CLI()
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tea.InitTerminal()
	var rootCmd cli.RootCmd
	rootCmd.RunWithSubcommands(rootCmd.AGPL())
}
