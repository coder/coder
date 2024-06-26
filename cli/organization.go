package cli

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) organizations() *serpent.Command {
	orgContext := NewOrganizationContext()

	cmd := &serpent.Command{
		Use:     "organizations [subcommand]",
		Short:   "Organization related commands",
		Aliases: []string{"organization", "org", "orgs"},
		Hidden:  true, // Hidden until these commands are complete.
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.showOrganization(orgContext),
			r.createOrganization(),
			r.organizationMembers(orgContext),
			r.organizationRoles(orgContext),
		},
	}

	orgContext.AttachOptions(cmd)
	return cmd
}

// promptUserSelectOrg will prompt the user to select an organization from a list
// of their organizations.
func promptUserSelectOrg(inv *serpent.Invocation, conf config.Root, orgs []codersdk.Organization) (string, error) {
	// Default choice
	var defaultOrg string
	// Comes from config file
	if conf.Organization().Exists() {
		defaultOrg, _ = conf.Organization().Read()
	}

	// No config? Comes from default org in the list
	if defaultOrg == "" {
		defIndex := slices.IndexFunc(orgs, func(org codersdk.Organization) bool {
			return org.IsDefault
		})
		if defIndex >= 0 {
			defaultOrg = orgs[defIndex].Name
		}
	}

	// Defer to first org
	if defaultOrg == "" && len(orgs) > 0 {
		defaultOrg = orgs[0].Name
	}

	// Ensure the `defaultOrg` value is an org name, not a uuid.
	// If it is a uuid, change it to the org name.
	index := slices.IndexFunc(orgs, func(org codersdk.Organization) bool {
		return org.ID.String() == defaultOrg || org.Name == defaultOrg
	})
	if index >= 0 {
		defaultOrg = orgs[index].Name
	}

	// deselectOption is the option to delete the organization config file and defer
	// to default behavior.
	const deselectOption = "[Default]"
	if defaultOrg == "" {
		defaultOrg = deselectOption
	}

	// Pull value from a prompt
	_, _ = fmt.Fprintln(inv.Stdout, pretty.Sprint(cliui.DefaultStyles.Wrap, "Select an organization below to set the current CLI context to:"))
	value, err := cliui.Select(inv, cliui.SelectOptions{
		Options:    append([]string{deselectOption}, orgNames(orgs)...),
		Default:    defaultOrg,
		Size:       10,
		HideSearch: false,
	})
	if err != nil {
		return "", err
	}
	// Deselect is an alias for ""
	if value == deselectOption {
		value = ""
	}

	return value, nil
}

// orgNames is a helper function to turn a list of organizations into a list of
// their names as strings.
func orgNames(orgs []codersdk.Organization) []string {
	names := make([]string, 0, len(orgs))
	for _, org := range orgs {
		names = append(names, org.Name)
	}
	return names
}

func (r *RootCmd) showOrganization(orgContext *OrganizationContext) *serpent.Command {
	var (
		stringFormat func(orgs []codersdk.Organization) (string, error)
		client       = new(codersdk.Client)
		formatter    = cliui.NewOutputFormatter(
			cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
				typed, ok := data.([]codersdk.Organization)
				if !ok {
					// This should never happen
					return "", xerrors.Errorf("expected []Organization, got %T", data)
				}
				return stringFormat(typed)
			}),
			cliui.TableFormat([]codersdk.Organization{}, []string{"id", "name", "default"}),
			cliui.JSONFormat(),
		)
		onlyID = false
	)
	cmd := &serpent.Command{
		Use: "show [\"selected\"|\"me\"|uuid|org_name]",
		Short: "Show the organization. " +
			"Using \"selected\" will show the selected organization from the \"--org\" flag. " +
			"Using \"me\" will show all organizations you are a member of.",
		Long: FormatExamples(
			Example{
				Description: "coder org show selected",
				Command: "Shows the organizations selected with '--org=<org_name>'. " +
					"This organization is the organization used by the cli.",
			},
			Example{
				Description: "coder org show me",
				Command:     "List of all organizations you are a member of.",
			},
			Example{
				Description: "coder org show developers",
				Command:     "Show organization with name 'developers'",
			},
			Example{
				Description: "coder org show 90ee1875-3db5-43b3-828e-af3687522e43",
				Command:     "Show organization with the given ID.",
			},
		),
		Middleware: serpent.Chain(
			r.InitClient(client),
			serpent.RequireRangeArgs(0, 1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "only-id",
				Description: "Only print the organization ID.",
				Required:    false,
				Flag:        "only-id",
				Value:       serpent.BoolOf(&onlyID),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			orgArg := "selected"
			if len(inv.Args) >= 1 {
				orgArg = inv.Args[0]
			}

			var orgs []codersdk.Organization
			var err error
			switch strings.ToLower(orgArg) {
			case "selected":
				stringFormat = func(orgs []codersdk.Organization) (string, error) {
					if len(orgs) != 1 {
						return "", xerrors.Errorf("expected 1 organization, got %d", len(orgs))
					}
					return fmt.Sprintf("Current CLI Organization: %s (%s)\n", orgs[0].Name, orgs[0].ID.String()), nil
				}
				org, err := orgContext.Selected(inv, client)
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
						return "", xerrors.Errorf("expected 1 organization, got %d", len(orgs))
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
