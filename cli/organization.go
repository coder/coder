package cli

import (
	"fmt"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) organizations() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "organizations { current }",
		Short:       "Organization related commands",
		Aliases:     []string{"organization", "org", "orgs"},
		Hidden:      true, // Hidden until these commands are complete.
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.currentOrganization(),
		},
	}

	cmd.Options = clibase.OptionSet{}
	return cmd
}

func (r *RootCmd) currentOrganization() *clibase.Cmd {
	var (
		client    = new(codersdk.Client)
		formatter = cliui.NewOutputFormatter(
			cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
				typed, ok := data.([]codersdk.Organization)
				if !ok {
					// This should never happen
					return "", fmt.Errorf("expected []Organization, got %T", data)
				}
				if len(typed) != 1 {
					return "", fmt.Errorf("expected 1 organization, got %d", len(typed))
				}
				return fmt.Sprintf("Current organization: %s (%s)\n", typed[0].Name, typed[0].ID.String()), nil
			}),
			cliui.TableFormat([]codersdk.Organization{}, []string{"id", "name", "default"}),
			cliui.JSONFormat(),
		)
		onlyID = false
	)
	cmd := &clibase.Cmd{
		Use:   "current",
		Short: "Show the current selected organization the cli will use.",
		Middleware: clibase.Chain(
			r.InitClient(client),
		),
		Options: clibase.OptionSet{
			{
				Name:        "only-id",
				Description: "Only print the organization ID.",
				Required:    false,
				Flag:        "only-id",
				Value:       clibase.BoolOf(&onlyID),
			},
		},
		Handler: func(inv *clibase.Invocation) error {
			org, err := CurrentOrganization(r, inv, client)
			if err != nil {
				return err
			}

			if onlyID {
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", org.ID)
			} else {
				out, err := formatter.Format(inv.Context(), []codersdk.Organization{org})
				if err != nil {
					return err
				}
				_, _ = fmt.Fprint(inv.Stdout, out)
			}
			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)

	return cmd
}
