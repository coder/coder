package cli

import "github.com/coder/serpent"

// **NOTE** Only covers site wide roles at present. Org scoped roles maybe
// should be nested under some command that scopes to an org??

func (r *RootCmd) roles() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "roles",
		Short:   "Manage roles",
		Aliases: []string{"role"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Hidden:   true,
		Children: []*serpent.Command{},
	}
	return cmd
}

func (r *RootCmd) showRole() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "show",
		Short: "Show role(s)",
		Handler: func(i *serpent.Invocation) error {

			return nil
		},
	}

	return cmd
}
