package clitest

import (
	"testing"

	"github.com/coder/coder/cli/clibase"
)

// HandlersOK asserts that all commands have a handler.
// Without a handler, the command has no default behavior. Even for
// non-root commands (like 'groups' or 'users'), a handler is required.
// These handlers are likely just the 'help' handler, but this must be
// explicitly set.
func HandlersOK(t *testing.T, cmd *clibase.Cmd) {
	cmd.Walk(func(cmd *clibase.Cmd) {
		if cmd.Handler == nil {
			// If you see this error, make the Handler a helper invoker.
			//   Handler: func(inv *clibase.Invocation) error {
			//	   return inv.Command.HelpHandler(inv)
			//	 },
			t.Errorf("command %q has no handler, change to a helper invoker using: 'inv.Command.HelpHandler(inv)'", cmd.Name())
		}
	})
}
