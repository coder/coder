package cli

import (
	"fmt"
	"strings"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) organizations() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "organizations [subcommand]",
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
		stringFormat func(orgs []codersdk.Organization) (string, error)
		client       = new(codersdk.Client)
		formatter    = cliui.NewOutputFormatter(
			cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
				typed, ok := data.([]codersdk.Organization)
				if !ok {
					// This should never happen
					return "", fmt.Errorf("expected []Organization, got %T", data)
				}
				return stringFormat(typed)
			}),
			cliui.TableFormat([]codersdk.Organization{}, []string{"id", "name", "default"}),
			cliui.JSONFormat(),
		)
		onlyID = false
	)
	cmd := &clibase.Cmd{
		Use:   "show [current|me|uuid]",
		Short: "Show the organization, if no argument is given, the organization currently in use will be shown.",
		Middleware: clibase.Chain(
			r.InitClient(client),
			clibase.RequireRangeArgs(0, 1),
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
			orgArg := "current"
			if len(inv.Args) >= 1 {
				orgArg = inv.Args[0]
			}

			var orgs []codersdk.Organization
			var err error
			switch strings.ToLower(orgArg) {
			case "current":
				stringFormat = func(orgs []codersdk.Organization) (string, error) {
					if len(orgs) != 1 {
						return "", fmt.Errorf("expected 1 organization, got %d", len(orgs))
					}
					return fmt.Sprintf("Current CLI Organization: %s (%s)\n", orgs[0].Name, orgs[0].ID.String()), nil
				}
				org, err := CurrentOrganization(r, inv, client)
				if err != nil {
					return err
				}
				orgs = []codersdk.Organization{org}
			case "me":
				stringFormat = func(orgs []codersdk.Organization) (string, error) {
					var str strings.Builder
					_, _ = fmt.Fprint(&str, "Organizations you are a member of:\n")
					for _, org := range orgs {
						_, _ = fmt.Fprintf(&str, "\t%s (%s)\n", org.Name, org.ID.String())
					}
					return str.String(), nil
				}
				orgs, err = client.OrganizationsByUser(inv.Context(), codersdk.Me)
				if err != nil {
					return err
				}
			default:
				stringFormat = func(orgs []codersdk.Organization) (string, error) {
					if len(orgs) != 1 {
						return "", fmt.Errorf("expected 1 organization, got %d", len(orgs))
					}
					return fmt.Sprintf("Organization: %s (%s)\n", orgs[0].Name, orgs[0].ID.String()), nil
				}
				// This works for a uuid or a name
				org, err := client.OrganizationByName(inv.Context(), orgArg)
				if err != nil {
					return err
				}
				orgs = []codersdk.Organization{org}
			}

			if onlyID {
				for _, org := range orgs {
					_, _ = fmt.Fprintf(inv.Stdout, "%s\n", org.ID)
				}
			} else {
				out, err := formatter.Format(inv.Context(), orgs)
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
