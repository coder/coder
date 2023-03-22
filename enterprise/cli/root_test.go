package cli_test

import (
	"strings"
	"testing"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/enterprise/cli"
)

func newCLI(t *testing.T, args ...string) (*clibase.Invocation, config.Root) {
	var root cli.RootCmd
	cmd := root.Command(root.EnterpriseSubcommands())
	return clitest.NewWithCommand(t, cmd, args...)
}

func TestRoot_LintUsage(t *testing.T) {
	// This command is in the enterprise package so that it captures every
	// single command.
	t.Parallel()

	var root cli.RootCmd
	cmd := root.Command(root.EnterpriseSubcommands())

	cmd.Walk(func(cmd *clibase.Cmd) {
		hasFlagsInUsage := strings.Index(cmd.Use, "[flags]") >= 0

		var hasFlag bool
		for _, opt := range cmd.Options {
			if opt.Flag != "" {
				hasFlag = true
				break
			}
		}

		if hasFlagsInUsage && !hasFlag {
			t.Errorf("command %q has [flags] in usage but no flags", cmd.FullUsage())
		}
		if !hasFlagsInUsage && hasFlag {
			t.Errorf("command %q has flags but no [flags] in usage", cmd.FullUsage())
		}
	})
}
