package exp

import (
	"github.com/coder/coder/cli/clibase"
)

type cmd struct {
	root *clibase.Cmd
}

// Cmd commands are primarily for internal use and may be
// subject to breaking changes without notice.
func Cmd(mw clibase.MiddlewareFunc) *clibase.Cmd {
	return &clibase.Cmd{
		Use:    "exp",
		Hidden: true,
		Short:  "Experimental commands. These are primarily for internal use and may be subject to breaking changes without notice.",
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			scaletest(mw),
		},
	}
}
