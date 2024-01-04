package cli

import (
	"fmt"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
)

// TODO(f0ssel): This should be removed a few versions after coder 2.7.0 has been released.
func (*RootCmd) templateCreate() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:    "create [name]",
		Short:  "Create a template from the current directory or as specified by flag",
		Hidden: true,
		Handler: func(inv *clibase.Invocation) error {
			_, _ = fmt.Fprintln(inv.Stdout, "\n"+pretty.Sprint(cliui.DefaultStyles.Wrap,
				pretty.Sprint(
					cliui.DefaultStyles.Error,
					"ERROR: The `coder templates create` command has been removed. "+
						"Use the `coder templates push` command to create and update templates. "+
						"Use the `coder templates edit` command to change template settings.")))
			return nil
		},
	}

	return cmd
}
