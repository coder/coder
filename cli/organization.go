package cli

import (
	"errors"
	"fmt"
	"os"
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
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "organizations [subcommand]",
		Short:       "Organization related commands",
		Aliases:     []string{"organization", "org", "orgs"},
		Hidden:      true, // Hidden until these commands are complete.
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.currentOrganization(),
			r.switchOrganization(),
			r.createOrganization(),
			r.organizationRoles(),
		},
	}

	cmd.Options = serpent.OptionSet{}
	return cmd
}

func (r *RootCmd) switchOrganization() *serpent.Command {
	client := new(codersdk.Client)

	cmd := &serpent.Command{
		Use:   "set <organization name | ID>",
		Short: "set the organization used by the CLI. Pass an empty string to reset to the default organization.",
		Long: "set the organization used by the CLI. Pass an empty string to reset to the default organization.\n" + FormatExamples(
			Example{
				Description: "Remove the current organization and defer to the default.",
				Command:     "coder organizations set ''",
			},
			Example{
				Description: "Switch to a custom organization.",
				Command:     "coder organizations set my-org",
			},
		),
		Middleware: serpent.Chain(
			r.InitClient(client),
			serpent.RequireRangeArgs(0, 1),
		),
		Options: serpent.OptionSet{},
		Handler: func(inv *serpent.Invocation) error {
			conf := r.createConfig()
			orgs, err := client.OrganizationsByUser(inv.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("failed to get organizations: %w", err)
			}
			// Keep the list of orgs sorted
			slices.SortFunc(orgs, func(a, b codersdk.Organization) int {
				return strings.Compare(a.Name, b.Name)
			})

			var switchToOrg string
			if len(inv.Args) == 0 {
				// Pull switchToOrg from a prompt selector, rather than command line
				// args.
				switchToOrg, err = promptUserSelectOrg(inv, conf, orgs)
				if err != nil {
					return err
				}
			} else {
				switchToOrg = inv.Args[0]
			}

			// If the user passes an empty string, we want to remove the organization
			// from the config file. This will defer to default behavior.
			if switchToOrg == "" {
				err := conf.Organization().Delete()
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return xerrors.Errorf("failed to unset organization: %w", err)
				}
				_, _ = fmt.Fprintf(inv.Stdout, "Organization unset\n")
			} else {
				// Find the selected org in our list.
				index := slices.IndexFunc(orgs, func(org codersdk.Organization) bool {
					return org.Name == switchToOrg || org.ID.String() == switchToOrg
				})
				if index < 0 {
					// Using this error for better error message formatting
					err := &codersdk.Error{
						Response: codersdk.Response{
							Message: fmt.Sprintf("Organization %q not found. Is the name correct, and are you a member of it?", switchToOrg),
							Detail:  "Ensure the organization argument is correct and you are a member of it.",
						},
						Helper: fmt.Sprintf("Valid organizations you can switch to: %s", strings.Join(orgNames(orgs), ", ")),
					}
					return err
				}

				// Always write the uuid to the config file. Names can change.
				err := conf.Organization().Write(orgs[index].ID.String())
				if err != nil {
					return xerrors.Errorf("failed to write organization to config file: %w", err)
				}
			}

			// Verify it worked.
			current, err := CurrentOrganization(r, inv, client)
			if err != nil {
				// An SDK error could be a permission error. So offer the advice to unset the org
				// and reset the context.
				var sdkError *codersdk.Error
				if errors.As(err, &sdkError) {
					if sdkError.Helper == "" && sdkError.StatusCode() != 500 {
						sdkError.Helper = `If this error persists, try unsetting your org with 'coder organizations set ""'`
					}
					return sdkError
				}
				return xerrors.Errorf("failed to get current organization: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Current organization context set to %s (%s)\n", current.Name, current.ID.String())
			return nil
		},
	}

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

func (r *RootCmd) currentOrganization() *serpent.Command {
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
		Use:   "show [current|me|uuid]",
		Short: "Show the organization, if no argument is given, the organization currently in use will be shown.",
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
						return "", xerrors.Errorf("expected 1 organization, got %d", len(orgs))
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
