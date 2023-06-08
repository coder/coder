package cli

import (
	"testing"

	"github.com/coder/coder/cli/clitest"
)

func TestEnterpriseCommandHelp(t *testing.T) {
	// Only test the enterprise commands
	clitest.TestCommandHelp(t, (&RootCmd{}).enterpriseOnly(),
		append(clitest.DefaultCases(),
			clitest.CommandHelpCase{
				Name: "coder wsproxy --help",
				Cmd:  []string{"wsproxy", "--help"},
			}),
	)
}
