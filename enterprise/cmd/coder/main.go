package main

import (
	"fmt"
	"os"
	_ "time/tzdata"

	"github.com/coder/coder/v2/agent/agentexec"
	entcli "github.com/coder/coder/v2/enterprise/cli"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "agent-exec" {
		err := agentexec.CLI()
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var rootCmd entcli.RootCmd
	rootCmd.RunWithSubcommands(rootCmd.EnterpriseSubcommands())
}
